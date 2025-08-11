package models

type SelfDeclaredAddressInput struct {
	Bairro         string  `json:"bairro" binding:"required"`
	CEP            string  `json:"cep" binding:"required"`
	Complemento    *string `json:"complemento"`
	Estado         string  `json:"estado" binding:"required"`
	Logradouro     string  `json:"logradouro" binding:"required"`
	Municipio      string  `json:"municipio" binding:"required"`
	Numero         string  `json:"numero" binding:"required"`
	TipoLogradouro *string `json:"tipo_logradouro"`
}

type SelfDeclaredEmailInput struct {
	Valor string `json:"valor" binding:"required"`
}

type SelfDeclaredPhoneInput struct {
	DDI string `json:"ddi" binding:"required"`
	// DDD é obrigatório somente quando o DDI é 55 (Brasil)
	DDD   string `json:"ddd" binding:"required_if=DDI 55"`
	Valor string `json:"valor" binding:"required"`
}

type SelfDeclaredRacaInput struct {
	Valor string `json:"valor" binding:"required"`
}
