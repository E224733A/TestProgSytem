package proto


const (

	// Partie 1 : Commandes envoyées par le.s client.s au serveur
	CommandeList = "List"
	CommandeGet = "Get"
	CommandeEnd = "End"

	// Partie 2 : Commandes envoyées par un client au serveur
	CommandeHide = "Hide"
	CommandeReveal = "Reveal"
	CommandeTerminate = "Terminate"

	// Réponses du serveur au.x client.s
	ReponseFileCount = "FileCnt"
	ReponseFileUnknown = "FileUnknown"
	ReponseStart = "Start"
	ReponseOk = "OK"

)