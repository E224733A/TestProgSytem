package client

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

func Run(remote string) {
	c, e := net.Dial("tcp", remote)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		c.Close()
		slog.Info("Connection closed")
	}()
	slog.Info("Connected to " + c.RemoteAddr().String())

	// Lire ce que l'utilisateur tape dans la console
	consoleScanner := bufio.NewScanner(os.Stdin)
	// Lire ce que le serveur envoie
	serverReader := bufio.NewReader(c)

	fmt.Println("Enter command (List, End, ...):")

	for consoleScanner.Scan() {
		input := consoleScanner.Text()
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}
		cmd := parts[0]

		// Envoi de la commande au serveur
		_, err := fmt.Fprintf(c, "%s\n", input)
		if err != nil {
			slog.Error("Failed to send command", "error", err)
			return
		}

		// Traitement spécifique de la réponse selon la commande
		switch cmd {
		case proto.CommandeList:
			gererListReponse(c, serverReader)
		case proto.CommandeEnd:
			return // On quitte le client
		}
	}
}

func gererListReponse(c net.Conn, reader *bufio.Reader) {
	// Lire "FileCnt N"
	line, err := reader.ReadString('\n')
	if err != nil {
		slog.Error("Error reading FileCnt", "error", err)
		return
	}
	line = strings.TrimSpace(line)

	var count int
	// Analyse la chaine "FileCnt N"
	if _, err := fmt.Sscanf(line, "%s %d", new(string), &count); err != nil {
		slog.Error("Invalid protocol format", "received", line)
		return
	}

	fmt.Printf("FileCnt %d\n", count)

	// Lire les N lignes de fichiers
	for i := 0; i < count; i++ {
		fileLine, err := reader.ReadString('\n')
		if err != nil {
			slog.Error("Error reading file info", "error", err)
			break
		}
		fmt.Print(" - " + fileLine)
	}

	// Envoyer OK
	fmt.Fprintf(c, "%s\n", proto.ReponseOk)
	slog.Debug("Sent OK confirmation")
}