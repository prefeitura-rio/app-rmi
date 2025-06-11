package models

type SelfDeclaredAddressInput struct {
	Bairro        string  `json:"bairro" binding:"required"`
	CEP           string  `json:"cep" binding:"required"`
	Complemento   *string `json:"complemento,omitempty"`
	Estado        string  `json:"estado" binding:"required"`
	Logradouro    string  `json:"logradouro" binding:"required"`
	Municipio     string  `json:"municipio" binding:"required"`
	Numero        string  `json:"numero" binding:"required"`
	TipoLogradouro *string `json:"tipo_logradouro,omitempty"`
}

type SelfDeclaredEmailInput struct {
	Valor string `json:"valor" binding:"required"`
}

type SelfDeclaredPhoneInput struct {
	DDI   string `json:"ddi" binding:"required"`
	DDD   string `json:"ddd" binding:"required"`
	Valor string `json:"valor" binding:"required"`
}

type SelfDeclaredRacaInput struct {
	Valor string `json:"valor" binding:"required"`
} 