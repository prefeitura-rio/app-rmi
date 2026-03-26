package models

// Bairro represents a neighborhood (bairro) record
type Bairro struct {
	ID   string `bson:"id_bairro" json:"id"`
	Nome string `bson:"nome" json:"nome"`
}

// BairroListResponse represents the response for listing bairros with pagination
type BairroListResponse struct {
	Bairros []Bairro `json:"bairros"`
	Total   int64    `json:"total"`
	Page    int      `json:"page"`
	Limit   int      `json:"limit"`
}

// BairroFilters represents the filters for querying bairros
type BairroFilters struct {
	Page   int
	Limit  int
	Search string
}
