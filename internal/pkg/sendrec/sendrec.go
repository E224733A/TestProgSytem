package sendrec

import (
	"bufio"
	"log/slog"
)

// SendMessage écrit un message (qui doit contenir un '\n') puis flush.
// Le serveur/client doivent s'assurer d'inclure le '\n' dans message.
func SendMessage(out *bufio.Writer, message string) error {

	_, err := out.WriteString(message)
	if err != nil {
		return err
	}
	if err := out.Flush(); err != nil {
		return err
	}

	// Pour le log : si message finit par '\n', on l'enlève
	if len(message) > 0 && message[len(message)-1] == '\n' {
		slog.Debug("Sending message", "msg", message[:len(message)-1])
	} else {
		slog.Debug("Sending message", "msg", message)
	}

	return nil
}

// ReceiveMessage lit une ligne terminée par '\n'
// et retourne la ligne SANS le '\n'
func ReceiveMessage(in *bufio.Reader) (string, error) {

	line, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}

	// retirer le '\n'
	line = line[:len(line)-1]

	slog.Debug("Received message", "msg", line)
	return line, nil
}
