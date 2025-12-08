package sendrec

import (
	"bufio"
	"log/slog"
)

func SendMessage(out *bufio.Writer, message string) error {
	_, err := out.WriteString(message)
	if err != nil {
		return err
	}
	err = out.Flush()
	if err != nil {
		return err
	}
	if len(message) > 0 && message[len(message)-1] == '\n' {
		slog.Debug("Sending message", "msg", message[:len(message)-1])
	} else {
		slog.Debug("Sending message", "msg", message)
	}
	return nil
}

func ReceiveMessage(in *bufio.Reader) (message string, err error) {
	message, err = in.ReadString('\n')
	if err != nil {
		return "", err
	}
	msg := message[0 : len(message)-1]
	slog.Debug("Received message", "msg", msg)
	return msg, nil
}
