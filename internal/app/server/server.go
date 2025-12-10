package server

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/sendrec"
)

// Messages pour gerer les fichiers caches via canal
type hideRequest struct {
	filename string
	response chan bool
}

type revealRequest struct {
	filename string
	response chan bool
}

type isHiddenRequest struct {
	filename string
	response chan bool
}

type listHiddenRequest struct {
	response chan map[string]bool
}

// Etat du serveur
type ServerState struct {
	shutdown chan struct{}
	wg       sync.WaitGroup
}

func gererClient(cnx net.Conn, nbClients chan int, dir string, hiddenManager chan interface{}, state *ServerState) {

	// On signale +1 client
	nbClients <- 1
	state.wg.Add(1)

	defer func() {
		nbClients <- -1
		state.wg.Done()
		cnx.Close()
		slog.Info("Connection closed", "client", cnx.RemoteAddr().String())
	}()

	slog.Info("New client connected", "client", cnx.RemoteAddr().String())

	reader := bufio.NewReader(cnx)
	writer := bufio.NewWriter(cnx)

	for {
		// Verifie si shutdown demande
		select {
		case <-state.shutdown:
			slog.Debug("Client handler shutting down", "client", cnx.RemoteAddr().String())
			return
		default:
		}

		// Timeout de 1s pour la lecture
		cnx.SetReadDeadline(time.Now().Add(1 * time.Second))

		// Lire la commande
		cmdLine, err := reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			slog.Error("Connection error", "error", err)
			return
		}

		// Enleve le timeout pour traiter la commande
		cnx.SetReadDeadline(time.Time{})

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
			commandList(reader, writer, dir, hiddenManager)

		case proto.CommandeGet:
			if len(parts) < 2 {
				slog.Warn("Get command missing filename")
				continue
			}
			filename := parts[1]
			commandGet(reader, writer, dir, filename, cnx.RemoteAddr().String(), hiddenManager)

		case proto.CommandeEnd:
			return

		default:
			slog.Warn("Unknown command", "command", cmd)
		}
	}
}

// --- CLIENT DE CONTRÔLE ---
func gererClientControle(cnx net.Conn, dir string, hiddenManager chan interface{}, state *ServerState) {
	defer func() {
		cnx.Close()
		slog.Info("Control connection closed", "client", cnx.RemoteAddr().String())
	}()

	slog.Info("Control client connected", "client", cnx.RemoteAddr().String())

	reader := bufio.NewReader(cnx)
	writer := bufio.NewWriter(cnx)

	for {
		// Lire la commande
		cmdLine, err := reader.ReadString('\n')
		if err != nil {
			slog.Error("Control connection error", "error", err)
			return
		}

		cmdLine = strings.TrimSpace(cmdLine)
		if cmdLine == "" {
			continue
		}

		parts := strings.Fields(cmdLine)
		cmd := parts[0]

		slog.Debug("Received control command",
			"command", cmd,
			"args", parts[1:],
			"from", cnx.RemoteAddr().String(),
		)

		switch cmd {

		case proto.CommandeList:
			commandList(reader, writer, dir, hiddenManager)

		case proto.CommandeHide:
			if len(parts) < 2 {
				slog.Warn("Hide command missing filename")
				continue
			}
			filename := parts[1]
			commandHide(writer, dir, filename, hiddenManager)

		case proto.CommandeReveal:
			if len(parts) < 2 {
				slog.Warn("Reveal command missing filename")
				continue
			}
			filename := parts[1]
			commandReveal(writer, dir, filename, hiddenManager)

		case proto.CommandeTerminate:
			commandTerminate(writer, state)
			return

		case proto.CommandeEnd:
			return

		default:
			slog.Warn("Unknown control command", "command", cmd)
		}
	}
}


// --- COMMANDE LIST ---
func commandList(reader *bufio.Reader, writer *bufio.Writer, dir string, hiddenManager chan interface{}) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Error("Failed to read directory", "error", err)
		return
	}

	// Recup la liste des fichiers caches
	req := listHiddenRequest{response: make(chan map[string]bool)}
	hiddenManager <- req
	hiddenFiles := <-req.response

	// Filtre pour ne garder que les fichiers non caches
	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && !hiddenFiles[e.Name()] {
			files = append(files, e)
		}
	}

	// Envoie FileCnt
	header := fmt.Sprintf("%s %d\n", proto.ReponseFileCount, len(files))
	if err := sendrec.SendMessage(writer, header); err != nil {
		slog.Error("Failed to send FileCnt", "error", err)
		return
	}

	// Envoie les noms + tailles
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

	// Attendre le OK du client
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

// --- COMMANDE GET ---
func commandGet(reader *bufio.Reader, writer *bufio.Writer, dir string, filename string, clientAddr string, hiddenManager chan interface{}) {
	// Verifie si le fichier est caché
	req := isHiddenRequest{filename: filename, response: make(chan bool)}
	hiddenManager <- req
	if <-req.response {
		slog.Warn("Attempt to get hidden file", "file", filename, "client", clientAddr)
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}

	// Construi le chemin complet du fichier
	filepath := dir + "/" + filename

	// Verifie si le fichier existe
	fileInfo, err := os.Stat(filepath)
	if err != nil {
		slog.Warn("File not found", "file", filename, "client", clientAddr)
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}

	// Verifie que c'est bien un fichier
	if fileInfo.IsDir() {
		slog.Warn("Requested path is a directory", "path", filename, "client", clientAddr)
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}

	// Ouvrir le fichier
	file, err := os.Open(filepath)
	if err != nil {
		slog.Error("Failed to open file", "file", filename, "error", err)
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}
	defer file.Close()

	// Envoie "Start <size>"
	startMsg := fmt.Sprintf("%s %d\n", proto.ReponseStart, fileInfo.Size())
	if err := sendrec.SendMessage(writer, startMsg); err != nil {
		slog.Error("Failed to send Start", "error", err)
		return
	}

	slog.Debug("Sending file", "file", filename, "size", fileInfo.Size(), "client", clientAddr)

	// Envoie le contenu du fichier (flux binaire)
	buffer := make([]byte, 4096)
	totalSent := int64(0)

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			written, writeErr := writer.Write(buffer[:n])
			if writeErr != nil {
				slog.Error("Failed to write file data", "error", writeErr)
				return
			}
			totalSent += int64(written)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			slog.Error("Error reading file", "error", err)
			return
		}
	}

	// Flush
	if err := writer.Flush(); err != nil {
		slog.Error("Failed to flush file data", "error", err)
		return
	}

	slog.Debug("File sent successfully", "file", filename, "bytes", totalSent)

	// Attendre OK
	resp, err := sendrec.ReceiveMessage(reader)
	if err != nil {
		slog.Error("Error waiting for client OK", "error", err)
		return
	}

	if resp == proto.ReponseOk {
		slog.Info("File transferred successfully", "file", filename, "size", totalSent, "client", clientAddr)
	} else {
		slog.Warn("Client did not send OK after file transfer", "received", resp, "file", filename)
	}
}

// --- COMMANDE HIDE ---
func commandHide(writer *bufio.Writer, dir string, filename string, hiddenManager chan interface{}) {
	// Verifie que le fichier existe dans le dossier
	filepath := dir + "/" + filename
	fileInfo, err := os.Stat(filepath)
	if err != nil || fileInfo.IsDir() {
		slog.Warn("Cannot hide file", "file", filename, "reason", "not found or is directory")
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}

	// Cacher le fichier via le canal
	req := hideRequest{filename: filename, response: make(chan bool)}
	hiddenManager <- req
	<-req.response

	slog.Info("File hidden", "file", filename)

	// Confirme
	if err := sendrec.SendMessage(writer, proto.ReponseOk+"\n"); err != nil {
		slog.Error("Failed to send OK", "error", err)
	}
}

// --- COMMANDE REVEAL ---
func commandReveal(writer *bufio.Writer, dir string, filename string, hiddenManager chan interface{}) {
	// Verfie que le fichier existe dans le dossier
	filepath := dir + "/" + filename
	fileInfo, err := os.Stat(filepath)
	if err != nil || fileInfo.IsDir() {
		slog.Warn("Cannot reveal file", "file", filename, "reason", "not found or is directory")
		if err := sendrec.SendMessage(writer, proto.ReponseFileUnknown+"\n"); err != nil {
			slog.Error("Failed to send FileUnknown", "error", err)
		}
		return
	}

	// Revele le fichier via le canal
	req := revealRequest{filename: filename, response: make(chan bool)}
	hiddenManager <- req
	wasHidden := <-req.response

	if wasHidden {
		slog.Info("File revealed", "file", filename)
	} else {
		slog.Debug("File was not hidden", "file", filename)
	}

	// Confirmer
	if err := sendrec.SendMessage(writer, proto.ReponseOk+"\n"); err != nil {
		slog.Error("Failed to send OK", "error", err)
	}
}

func commandTerminate(writer *bufio.Writer, state *ServerState) {
	slog.Info("Terminate command received - initiating server shutdown")

	// Signale l'arret a tous les clients
	close(state.shutdown)

	// Laisser un temps au client pour recevoir le signal
	time.Sleep(100 * time.Millisecond)

	// Attend les clients qui se deconnectent
	slog.Info("Waiting for all clients to disconnect...")
	
	// Timeout de 5s qui evite de bloquer indefiniment
	done := make(chan struct{})
	go func() {
		state.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("All clients disconnected")
	case <-time.After(5 * time.Second):
		slog.Warn("Timeout waiting for clients to disconnect, forcing shutdown")
	}

	// Confirmer
	if err := sendrec.SendMessage(writer, proto.ReponseOk+"\n"); err != nil {
		slog.Error("Failed to send OK", "error", err)
	}

	slog.Info("Server shutdown complete")
}

func RunServer(port *string, controlPort *string, dir *string) {

	nbClients := make(chan int)
	hiddenManager := make(chan interface{})
	state := &ServerState{
		shutdown: make(chan struct{}),
	}	

	// Compteur clients
	go func() {
		nb := 0
		for c := range nbClients {
			nb += c
			slog.Info("Clients connectés", slog.Int("count", nb))
		}
	}()

	// Gestionnaire des fichiers caches
	go func() {
		hiddenFiles := make(map[string]bool)
		for req := range hiddenManager {
			switch r := req.(type) {
			case hideRequest:
				hiddenFiles[r.filename] = true
				r.response <- true

			case revealRequest:
				wasHidden := hiddenFiles[r.filename]
				delete(hiddenFiles, r.filename)
				r.response <- wasHidden

			case isHiddenRequest:
				r.response <- hiddenFiles[r.filename]

			case listHiddenRequest:
				copy := make(map[string]bool)
				for k, v := range hiddenFiles {
					copy[k] = v
				}
				r.response <- copy
			}
		}
	}()

	// Ecoute reseau principal
	l, e := net.Listen("tcp", ":"+*port)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		l.Close()
		slog.Debug("Stopped listening on port " + *port)
	}()

	// Ecoute reseau controle
	lControl, e := net.Listen("tcp", ":"+*controlPort)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		lControl.Close()
		slog.Debug("Stopped listening on control port " + *controlPort)
	}()

	slog.Info("Server listening on port "+*port,
		"control_port", *controlPort,
		"directory", *dir)

	// Goroutine pour le port de controle (un seul possible)
	go func() {
		for {
			select {
			case <-state.shutdown:
				return
			default:
			}

			cnx, e := lControl.Accept()
			if e != nil {
				select {
				case <-state.shutdown:
					return
				default:
					slog.Error("Control accept error", "error", e)
					continue
				}
			}

			// Gere le client de controle (un seul possible)
			gererClientControle(cnx, *dir, hiddenManager, state)

			// Si la commande Terminate est execute, alors la gouroutine s'arrete
			select {
			case <-state.shutdown:
				return
			default:
			}
		}
	}()

	// Boucle d'acceptation des clients normaux
	for {
		select {
		case <-state.shutdown:
			slog.Info("Main listener shutting down")
			return
		default:
		}

		cnx, e := l.Accept()
		if e != nil {
			select {
			case <-state.shutdown:
				return
			default:
				slog.Error(e.Error())
				continue
			}
		}

		go gererClient(cnx, nbClients, *dir, hiddenManager, state)
	}	
}