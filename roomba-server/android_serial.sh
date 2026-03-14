#!/bin/bash
TTYDIR="../Termux-serial-tty"
USB_DEV=$(termux-usb -l | tr -d '[]" \n' | tr ',' '\n' | head -1)

if [ -z "$USB_DEV" ]; then
    echo "USB device not found"
    exit 1
fi

echo "USB device: $USB_DEV"

echo "Requesting USB permission..."
termux-usb -r "$USB_DEV"
sleep 1

pkill -f ptyserial 2>/dev/null
sleep 0.5

# ptyserialをバックグラウンドで起動
termux-usb -e "env LD_LIBRARY_PATH=$TTYDIR/bin $TTYDIR/ptyserial 115200" "$USB_DEV" 2>./ptyserial.log &
PTYSERIAL_PID=$!

# PTYパスが出るまで待つ
PTY=""
for i in $(seq 1 10); do
    PTY=$(grep "PTY created" ./ptyserial.log | awk '{print $3}')
    [ -n "$PTY" ] && break
    sleep 0.5
done

if [ -z "$PTY" ]; then
    echo "PTY not found"
    kill $PTYSERIAL_PID
    exit 1
fi

echo "PTY: $PTY"

# GoアプリにPTYパスを渡して起動
go build -o roomba-server .
./roomba-server "$PTY"

# 終了時にptyserialも止める
kill $PTYSERIAL_PID
