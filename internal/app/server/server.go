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
		// On signale -1 client
		nbClients <- -1
		cnx.Close()
		slog.Info("Connection closed", "client", cnx.RemoteAddr().String())
	}()

	slog.Info("New client connected", "client", cnx.RemoteAddr().String())

	reader := bufio.NewReader(cnx)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		cmdLine := scanner.Text()
		parts := strings.Fields(cmdLine) // En prevoyance de la commande "Get"
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		slog.Debug("Received command", "command", cmd, "args", parts[1:], "from", cnx.RemoteAddr().String())

		switch cmd {
		case proto.CommandeList:
			commandList(cnx, dir, reader)
		case proto.CommandeEnd:
			return // Ferme la connexion
		default:
			// Commande inconnue
			slog.Warn("Unknown command", "command", cmd)
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Error("Connection error", "error", err)
	}
}

// --- COMMANDE LISTE ---
func commandList(cnx net.Conn, dir string, reader *bufio.Reader) {
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

	// Envoyer FileCnt
	fmt.Fprintf(cnx, "%s %d\n", proto.ReponseFileCount, len(files))

	// Envoyer les infos de chaque fichier
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(cnx, "%s %d\n", f.Name(), info.Size())
	}

	// Attendre le "OK" du client
	// Utilise reader.ReadString pour etre sur de lire jusqu'au saut de ligne
	resp, err := reader.ReadString('\n')
	if err != nil {
		slog.Error("Error waiting for OK from client", "error", err)
		return
	}
	resp = strings.TrimSpace(resp)

	if resp == proto.ReponseOk {
		slog.Debug("Client confirmed reception of list")
	} else {
		slog.Warn("Client did not send OK", "received", resp)
	}
}

func RunServer(port *string, dir *string) {

	nbClients := make(chan int)

	// Compteur de client en temps réel avec une go routine qui compte via le canal
	go func() {
		nb := 0
		for c := range nbClients {
			nb += c
			slog.Info("Clients connectés", slog.Int("count", nb))
		}
	}()

	// Partie d'écoute du réseau
	l, e := net.Listen("tcp", ":"+*port)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		l.Close()
		slog.Debug("Stopped listening on port " + *port)
	}()

	slog.Info("Server listening on port " + *port)
	slog.Info("Serving files from directory: " + *dir)

	// Gestion des clients
	for {
		cnx, e := l.Accept()
		if e != nil {
			slog.Error(e.Error())
			continue
		}
		// go routines pour gérer plusieurs clients
		go gererClient(cnx, nbClients, *dir)
	}
}
