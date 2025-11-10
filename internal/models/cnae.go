package models

// CNAE represents a service classification (Classificação Nacional de Atividades Econômicas)
type CNAE struct {
	ID          string `bson:"_id" json:"id"`
	Secao       string `bson:"Secao" json:"secao"`
	Divisao     string `bson:"Divisao" json:"divisao"`
	Grupo       string `bson:"Grupo" json:"grupo"`
	Classe      string `bson:"Classe" json:"classe"`
	Subclasse   string `bson:"Subclasse" json:"subclasse"`
	Denominacao string `bson:"Denominacao" json:"denominacao"`
}

// CNAEListResponse represents the response for listing CNAEs with pagination
type CNAEListResponse struct {
	CNAEs      []CNAE         `json:"cnaes"`
	Pagination PaginationInfo `json:"pagination"`
}

// CNAEFilters represents the filters for querying CNAEs
type CNAEFilters struct {
	Page      int
	PerPage   int
	Search    string
	Secao     string
	Divisao   string
	Grupo     string
	Classe    string
	Subclasse string
}
