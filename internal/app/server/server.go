package server

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/sendrec"
)

func gererClient(cnx net.Conn, nbClients chan int, dir string) {

	// On signale +1 client
	nbClients <- 1

	defer func() {
		nbClients <- -1
		cnx.Close()
		slog.Info("Connection closed", "client", cnx.RemoteAddr().String())
	}()

	slog.Info("New client connected", "client", cnx.RemoteAddr().String())

	reader := bufio.NewReader(cnx)
	writer := bufio.NewWriter(cnx)

	for {
		// Lire la commande
		cmdLine, err := reader.ReadString('\n')
		if err != nil {
			slog.Error("Connection error", "error", err)
			return
		}

		cmdLine = strings.TrimSpace(cmdLine)
		if cmdLine == "" {
			continue
		}

		parts := strings.Fields(cmdLine)
		cmd := parts[0]

		slog.Debug("Received command",
			"command", cmd,
			"args", parts[1:],
			"from", cnx.RemoteAddr().String(),
		)

		switch cmd {

		case proto.CommandeList:
			commandList(reader, writer, dir)

		case proto.CommandeEnd:
			return

		default:
			slog.Warn("Unknown command", "command", cmd)
		}
	}
}

// --- COMMANDE LIST ---
// Utilise sendrec pour envoyer toutes les lignes du protocole.
func commandList(reader *bufio.Reader, writer *bufio.Writer, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Error("Failed to read directory", "error", err)
		return
	}

	// Filtrer pour ne garder que les fichiers
	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e)
		}
	}

	// 1) Envoyer FileCnt + '\n'
	header := fmt.Sprintf("%s %d\n", proto.ReponseFileCount, len(files))
	if err := sendrec.SendMessage(writer, header); err != nil {
		slog.Error("Failed to send FileCnt", "error", err)
		return
	}

	// 2) Envoyer les noms + tailles, chacun terminé par '\n'
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			slog.Warn("Could not stat file", "file", f.Name(), "error", err)
			continue
		}

		line := fmt.Sprintf("%s %d\n", f.Name(), info.Size())
		if err := sendrec.SendMessage(writer, line); err != nil {
			slog.Error("Failed to send file info", "file", f.Name(), "error", err)
			return
		}
	}

	// 3) Attendre le OK du client
	resp, err := sendrec.ReceiveMessage(reader)
	if err != nil {
		slog.Error("Error waiting for client OK", "error", err)
		return
	}

	if resp == proto.ReponseOk {
		slog.Debug("Client confirmed reception of list")
	} else {
		slog.Warn("Client did not send OK", "received", resp)
	}
}

func RunServer(port *string, dir *string) {

	nbClients := make(chan int)

	// Compteur clients
	go func() {
		nb := 0
		for c := range nbClients {
			nb += c
			slog.Info("Clients connectés", slog.Int("count", nb))
		}
	}()

	// Écoute réseau
	l, e := net.Listen("tcp", ":"+*port)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		l.Close()
		slog.Debug("Stopped listening on port " + *port)
	}()

	slog.Info("Server listening on port "+*port,
		"directory", *dir)

	// Boucle d’acceptation
	for {
		cnx, e := l.Accept()
		if e != nil {
			slog.Error(e.Error())
			continue
		}

		go gererClient(cnx, nbClients, *dir)
	}
}
