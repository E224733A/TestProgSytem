package main

import (
	"flag"
	"log/slog"
	"os"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/app/server"
)

func parseArgs() (port *string, dir *string) {

	logLevel := flag.Bool("d", false, "enable debug log level")
	port = flag.String("p", "3333", "server port (default: 3333)")
	// Parametre pour le dossier
	dir = flag.String("dir", ".", "directory to serve (default: .)")

	flag.Parse()

	if *logLevel {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Set logging level to debug")
	}

	// Verfie si le dossier existe
	if info, err := os.Stat(*dir); err != nil || !info.IsDir() {
		slog.Error("The specified directory does not exist or is not a directory: " + *dir)
		os.Exit(1)
	}

	return
}

func main() {
	port, dir := parseArgs()
	// Ajout du dossier au serveur
	server.RunServer(port, dir)
}