package models

// TelefoneUpdate represents the update structure for a phone number
// swagger:model
type TelefoneUpdate struct {
	// DDD (area code) must be exactly 2 digits
	// example: "21"
	// min: 2 max: 2
	DDD *string `json:"ddd,omitempty" binding:"omitempty,len=2,numeric"`

	// Phone number must be 8 or 9 digits
	// example: "999887766"
	// min: 8 max: 9
	Numero *string `json:"numero,omitempty" binding:"omitempty,min=8,max=9,numeric"`

	// Phone type must be one of: RESIDENCIAL, COMERCIAL, CELULAR
	// example: "CELULAR"
	// enum: RESIDENCIAL,COMERCIAL,CELULAR
	Tipo *string `json:"tipo,omitempty" binding:"omitempty,oneof=RESIDENCIAL COMERCIAL CELULAR"`

	// Optional notes about the phone number
	// example: "Horário comercial"
	// max: 200
	Observacoes *string `json:"observacoes,omitempty" binding:"omitempty,max=200"`
}

// EmailUpdate represents the update structure for an email address
// swagger:model
type EmailUpdate struct {
	// Valid email address
	// example: "user@example.com"
	Email *string `json:"email,omitempty" binding:"omitempty,email"`

	// Email type must be one of: PESSOAL, COMERCIAL
	// example: "PESSOAL"
	// enum: PESSOAL,COMERCIAL
	Tipo *string `json:"tipo,omitempty" binding:"omitempty,oneof=PESSOAL COMERCIAL"`

	// Optional notes about the email
	// example: "Email principal"
	// max: 200
	Observacoes *string `json:"observacoes,omitempty" binding:"omitempty,max=200"`
}

// ContatoUpdate represents the update structure for contact information
// swagger:model
type ContatoUpdate struct {
	// List of phone numbers
	Telefones []TelefoneUpdate `json:"telefones,omitempty" binding:"omitempty,dive"`

	// List of email addresses
	Emails []EmailUpdate `json:"emails,omitempty" binding:"omitempty,dive"`
}

// EnderecoUpdate represents the update structure for an address
// swagger:model
type EnderecoUpdate struct {
	// Street name
	// example: "Rua das Flores"
	// min: 3 max: 200
	Logradouro *string `json:"logradouro,omitempty" binding:"omitempty,min=3,max=200"`

	// Street number
	// example: "123"
	// max: 10
	Numero *string `json:"numero,omitempty" binding:"omitempty,max=10"`

	// Additional address information
	// example: "Apto 101"
	// max: 100
	Complemento *string `json:"complemento,omitempty" binding:"omitempty,max=100"`

	// Neighborhood
	// example: "Centro"
	// min: 3 max: 100
	Bairro *string `json:"bairro,omitempty" binding:"omitempty,min=3,max=100"`

	// City name
	// example: "Rio de Janeiro"
	// min: 3 max: 100
	Cidade *string `json:"cidade,omitempty" binding:"omitempty,min=3,max=100"`

	// State abbreviation (2 letters)
	// example: "RJ"
	// min: 2 max: 2
	UF *string `json:"uf,omitempty" binding:"omitempty,len=2"`

	// ZIP code (8 digits)
	// example: "20000000"
	// min: 8 max: 8
	CEP *string `json:"cep,omitempty" binding:"omitempty,len=8,numeric"`

	// Address type must be one of: RESIDENCIAL, COMERCIAL
	// example: "RESIDENCIAL"
	// enum: RESIDENCIAL,COMERCIAL
	TipoEndereco *string `json:"tipo_endereco,omitempty" binding:"omitempty,oneof=RESIDENCIAL COMERCIAL"`
}

// CadastroUpdate represents the update structure for self-declared data
// swagger:model
type CadastroUpdate struct {
	// Address information
	Endereco *EnderecoUpdate `json:"endereco,omitempty" binding:"omitempty"`

	// Contact information
	Contato *ContatoUpdate `json:"contato,omitempty" binding:"omitempty"`
}

// ToSelfDeclaredData converts CadastroUpdate to SelfDeclaredData
func (c *CadastroUpdate) ToSelfDeclaredData() *SelfDeclaredData {
	data := &SelfDeclaredData{}

	if c.Endereco != nil {
		data.Endereco = &Endereco{
			Logradouro:   c.Endereco.Logradouro,
			Numero:       c.Endereco.Numero,
			Complemento:  c.Endereco.Complemento,
			Bairro:       c.Endereco.Bairro,
			Cidade:       c.Endereco.Cidade,
			UF:           c.Endereco.UF,
			CEP:          c.Endereco.CEP,
			TipoEndereco: c.Endereco.TipoEndereco,
		}
	}

	if c.Contato != nil {
		data.Contato = &Contato{}
		
		if len(c.Contato.Telefones) > 0 {
			data.Contato.Telefones = make([]Telefone, len(c.Contato.Telefones))
			for i, tel := range c.Contato.Telefones {
				data.Contato.Telefones[i] = Telefone{
					DDD:         tel.DDD,
					Numero:      tel.Numero,
					Tipo:        tel.Tipo,
					Observacoes: tel.Observacoes,
				}
			}
		}

		if len(c.Contato.Emails) > 0 {
			data.Contato.Emails = make([]Email, len(c.Contato.Emails))
			for i, email := range c.Contato.Emails {
				data.Contato.Emails[i] = Email{
					Email:       email.Email,
					Tipo:        email.Tipo,
					Observacoes: email.Observacoes,
				}
			}
		}
	}

	return data
} 