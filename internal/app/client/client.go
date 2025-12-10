package client

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
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

	for {		
		// Lire la commande
		if !consoleScanner.Scan() {
			break
		}
		
		input := consoleScanner.Text()
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}
		cmd := parts[0]

		// Envoie de la commande au serveur
		_, err := fmt.Fprintf(c, "%s\n", input)
		if err != nil {
			slog.Error("Failed to send command", "error", err)
			return
		}

		// Traite specifiquement la reponse selon la commande
		switch cmd {
		case proto.CommandeList:
			gererListReponse(c, serverReader)
		case proto.CommandeGet:
			if len(parts) < 2 {
				fmt.Println("Usage: Get <filename>")
				continue
			}
			filename := parts[1]
			gererGetReponse(c, serverReader, filename)
		case proto.CommandeEnd:
			return
		default:
			fmt.Printf("Unknown command: %s\n", cmd)
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

	// Analyse la chaine "FileCnt N"
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != proto.ReponseFileCount {
		slog.Error("Invalid protocol format", "received", line)
		return
	}

	count, err := strconv.Atoi(parts[1])
	if err != nil {
		slog.Error("Invalid file count", "received", parts[1])
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

	// Envoie OK
	fmt.Fprintf(c, "%s\n", proto.ReponseOk)
	slog.Debug("Sent OK confirmation")
}

func gererGetReponse(c net.Conn, reader *bufio.Reader, filename string) {
	// Lire la premiere ligne de reponse du serveur
	line, err := reader.ReadString('\n')
	if err != nil {
		slog.Error("Error reading server response", "error", err)
		return
	}
	line = strings.TrimSpace(line)

	// Verifie si c'est FileUnknown
	if line == proto.ReponseFileUnknown {
		fmt.Printf("Error: File '%s' not found on server\n", filename)
		return
	}

	// "Start <size>"
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != proto.ReponseStart {
		slog.Error("Unexpected response from server", "received", line)
		return
	}

	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		slog.Error("Invalid file size in response", "received", parts[1])
		return
	}

	fmt.Printf("Downloading '%s' (%d bytes)...\n", filename, size)

	// Creer le fichier localement
	outFile, err := os.Create(filename)
	if err != nil {
		slog.Error("Failed to create local file", "file", filename, "error", err)
		return
	}
	defer outFile.Close()

	// Lit exactement 'size' octets
	buffer := make([]byte, 4096)
	totalReceived := int64(0)

	for totalReceived < size {
		remaining := size - totalReceived
		toRead := int64(len(buffer))
		if remaining < toRead {
			toRead = remaining
		}

		n, err := reader.Read(buffer[:toRead])
		if n > 0 {
			written, writeErr := outFile.Write(buffer[:n])
			if writeErr != nil {
				slog.Error("Failed to write to file", "error", writeErr)
				return
			}
			totalReceived += int64(written)
		}

		if err != nil {
			slog.Error("Error reading file data", "error", err, "received", totalReceived, "expected", size)
			return
		}
	}

	fmt.Printf("File '%s' downloaded successfully (%d bytes)\n", filename, totalReceived)

	// Envoie OK au serveur
	fmt.Fprintf(c, "%s\n", proto.ReponseOk)
	slog.Debug("Sent OK confirmation for file transfer")
}