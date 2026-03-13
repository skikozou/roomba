package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type LogUI struct {
	app        *tview.Application
	logView    *tview.TextView
	inputField *tview.InputField
	flex       *tview.Flex

	// 入力制御用
	inputEnabled bool
	inputPrompt  string
	inputChan    chan string
	inputHistory []string
	historyIndex int
	historyDraft string
	outputOnly   bool
	mu           sync.Mutex
}

func NewLogUI() *LogUI {
	ui := &LogUI{
		app:          tview.NewApplication(),
		inputEnabled: false,
		inputChan:    make(chan string, 1),
		historyIndex: -1,
		outputOnly:   false,
	}

	ui.setupUI()
	return ui
}

func (ui *LogUI) setupUI() {
	// 上部ログ表示用のTextView
	ui.logView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWordWrap(true).
		SetWrap(true).
		SetChangedFunc(func() {
			ui.app.Draw()
		})
	ui.logView.SetBorder(true)

	// 下部入力用のInputField
	ui.inputField = tview.NewInputField().
		SetLabel("> ").
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetFieldTextColor(tcell.ColorWhite).
		SetLabelColor(tcell.ColorWhite)
	ui.inputField.SetBorder(true)
	ui.inputField.SetBackgroundColor(tcell.ColorBlack)

	// 初期状態では入力を無効化
	ui.inputField.SetDisabled(true)

	// 入力完了時の処理
	ui.inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			ui.handleInput()
		}
	})

	// 履歴移動（↑/↓）
	ui.inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			ui.historyBack()
			return nil
		case tcell.KeyDown:
			ui.historyForward()
			return nil
		default:
			return event
		}
	})

	// フォーカス移動キーを無効化してInputFieldに固定する
	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTAB, tcell.KeyBacktab:
			ui.app.SetFocus(ui.inputField)
			return nil
		default:
			return event
		}
	})

	// レイアウト作成
	ui.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ui.logView, 0, 1, false).
		AddItem(ui.inputField, 3, 1, true)
}

func (ui *LogUI) handleInput() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if !ui.inputEnabled {
		return
	}

	text := ui.inputField.GetText()
	if strings.TrimSpace(text) != "" {
		ui.inputHistory = append(ui.inputHistory, text)
		ui.historyIndex = -1
		ui.historyDraft = ""

		// 接続確立後のoutput-onlyモードでは入力をログ表示しない。
		if !ui.outputOnly {
			// 入力をログに表示（TextViewは任意のgoroutineから安全に書き込み可能）
			fmt.Fprintf(ui.logView, "[white]%s: %s[white]\n", ui.inputPrompt, text)
			ui.logView.ScrollToEnd()
		}

		// 入力欄をクリア
		ui.inputField.SetText("")

		// 入力を無効化
		ui.inputEnabled = false
		ui.inputField.SetDisabled(true)
		ui.inputField.SetLabel("> ")
		ui.app.SetFocus(ui.inputField)

		// チャンネルに送信
		go func() {
			ui.inputChan <- text
		}()
	}
}

func (ui *LogUI) enableInput(prompt string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ui.inputEnabled = true
	ui.inputPrompt = prompt
	ui.historyIndex = -1
	ui.historyDraft = ""

	ui.inputField.SetDisabled(false)
	if ui.outputOnly {
		ui.inputField.SetLabel(fmt.Sprintf("%s > ", prompt))
	} else {
		ui.inputField.SetLabel("> ")
	}
	ui.app.SetFocus(ui.inputField)
	ui.app.Draw() // Draw()は任意のgoroutineから安全に呼び出し可能
}

func (ui *LogUI) disableInput() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ui.inputEnabled = false
	ui.inputField.SetDisabled(true)
	ui.inputField.SetLabel("> ")
	ui.app.SetFocus(ui.inputField)
	ui.app.Draw()
}

func (ui *LogUI) historyBack() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if !ui.inputEnabled || len(ui.inputHistory) == 0 {
		return
	}

	if ui.historyIndex == -1 {
		ui.historyDraft = ui.inputField.GetText()
		ui.historyIndex = len(ui.inputHistory) - 1
	} else if ui.historyIndex > 0 {
		ui.historyIndex--
	}

	ui.inputField.SetText(ui.inputHistory[ui.historyIndex])
}

func (ui *LogUI) historyForward() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if !ui.inputEnabled || len(ui.inputHistory) == 0 || ui.historyIndex == -1 {
		return
	}

	if ui.historyIndex < len(ui.inputHistory)-1 {
		ui.historyIndex++
		ui.inputField.SetText(ui.inputHistory[ui.historyIndex])
		return
	}

	ui.historyIndex = -1
	ui.inputField.SetText(ui.historyDraft)
}

func (ui *LogUI) SetOutputOnly(enabled bool) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.outputOnly = enabled
}

func (ui *LogUI) isOutputOnly() bool {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.outputOnly
}

func (ui *LogUI) writeLog(color, message string) {
	// 改行文字を適切に処理
	cleanMessage := strings.ReplaceAll(message, "\r\n", "\n")
	cleanMessage = strings.ReplaceAll(cleanMessage, "\r", "\n")
	fmt.Fprintf(ui.logView, "%s%s[white]\n", color, cleanMessage)
	ui.logView.ScrollToEnd()
}

// ログにメッセージを追加（TextViewは任意のgoroutineから安全）
func (ui *LogUI) Log(message string) {
	if ui.isOutputOnly() {
		return
	}
	ui.writeLog("[yellow]", message)
}

// 相手側からの出力ログ（output-onlyモードでも表示）
func (ui *LogUI) LogOutput(message string) {
	ui.writeLog("[yellow]", message)
}

// ローカルコマンドの応答ログ（output-onlyモードでも表示）
func (ui *LogUI) LogCommand(message string) {
	ui.writeLog("[cyan]", message)
}

// シリアルポートからの生データ用のログメソッド
func (ui *LogUI) LogRaw(data []byte) {
	// バイナリデータを安全に表示
	output := ""
	for _, b := range data {
		if b >= 32 && b <= 126 { // 印刷可能文字
			output += string(b)
		} else if b == '\n' {
			output += "\n"
		} else if b == '\r' {
			// CRは無視（Windows改行対応）
			continue
		} else {
			output += fmt.Sprintf("\\x%02x", b)
		}
	}

	if strings.TrimSpace(output) != "" {
		fmt.Fprintf(ui.logView, "[cyan]RX: %s[white]\n", output)
		ui.logView.ScrollToEnd()
	}
}

// 入力を求める（ブロッキング）
func (ui *LogUI) RequestInput(prompt string) string {
	ui.Log(fmt.Sprintf("[green]%s", prompt))
	ui.enableInput(prompt)

	// 入力を待機
	input := <-ui.inputChan
	return input
}

// UIを開始
func (ui *LogUI) Run() error {
	return ui.app.SetRoot(ui.flex, true).EnableMouse(false).Run()
}

// UIを停止
func (ui *LogUI) Stop() {
	ui.app.Stop()
}

// UIを完全終了（context連携）
func (ui *LogUI) Shutdown() {
	// RequestInputでブロックしているgoroutineを解放
	ui.mu.Lock()
	if ui.inputEnabled {
		ui.inputEnabled = false
	}
	ui.mu.Unlock()

	// inputChanを空にして詰まりを解消
	select {
	case <-ui.inputChan:
	default:
	}

	// inputChanをcloseして<-ui.inputChanで待っているRequestInputを解放
	close(ui.inputChan)

	ui.app.Stop()
}
