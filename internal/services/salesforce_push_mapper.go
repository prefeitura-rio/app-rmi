package services

import (
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// buildSalesforceSnapshotPatch mirrors RMI state to Salesforce.
// Cleared fields in an existing Mongo document are sent as JSON null.
// Fields absent from Mongo are omitted so Salesforce keeps its current value.
func buildSalesforceSnapshotPatch(citizen *models.Citizen, sd *models.SelfDeclaredData, uc *models.UserConfig, citizenFound, selfDeclaredFound bool) *clients.SalesforceCidadaoPatchRequest {
	patch := clients.NewSalesforceSnapshotPatch()

	if citizenFound {
		putString(patch, "nome", rmiNome(citizen))
		putString(patch, "nomeSocial", rmiNomeSocial(citizen))
		putString(patch, "primeiroNome", rmiPrimeiroNome(citizen))
		putString(patch, "dataNascimento", rmiDataNascimento(citizen))
	}

	if selfDeclaredFound {
		putString(patch, "nomeExibicao", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.NomeExibicao }))
		putString(patch, "genero", rmiGenero(sd))
		putString(patch, "escolaridade", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.Escolaridade }))
		putString(patch, "rendaFamiliar", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.RendaFamiliar }))
		putString(patch, "deficiencia", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.Deficiencia }))
		putString(patch, "nacionalidade", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.Nacionalidade }))
		putString(patch, "passaporte", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.Passaporte }))
		putString(patch, "complemento", rmiSelfDeclaredComplemento(sd))
		putString(patch, "tipoTelefone1", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.TipoTelefone1 }))
		putString(patch, "tipoTelefone2", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.TipoTelefone2 }))
		putString(patch, "tipoTelefone3", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.TipoTelefone3 }))
		putString(patch, "telefoneInternacional", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.TelefoneInternacional }))
		putString(patch, "canalOrigem", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.CanalOrigem }))
		putString(patch, "canalUltimaModificacao", rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.CanalUltimaModificacao }))
		putString(patch, "telefone2", rmiTelefoneAlternativo(sd, 0))
		putString(patch, "telefone3", rmiTelefoneAlternativo(sd, 1))
		putIdioma(patch, sd, selfDeclaredFound)
		putIsTourist(patch, sd, selfDeclaredFound)
	}

	putConsentimento(patch, uc)

	putString(patch, "email", rmiEmail(sd, citizen, selfDeclaredFound))
	tel1 := rmiTelefonePrincipal(sd, citizen, selfDeclaredFound)
	putString(patch, "telefone1", tel1)
	putString(patch, "telefonePrincipal", tel1)
	putString(patch, "raca", rmiRaca(sd, citizen, selfDeclaredFound, citizenFound))
	putEndereco(patch, sd, citizen, selfDeclaredFound, citizenFound)

	if cidade := rmiCidade(sd, citizen, selfDeclaredFound, citizenFound); cidade.set {
		if cidade.clear {
			patch.PutClear("cidade")
		} else {
			patch.Put("cidade", cidade.value)
		}
	}

	patch.Put("contaOrigem", SalesforceContaOrigem)
	return patch
}

func buildSalesforceCreate(citizen *models.Citizen, sd *models.SelfDeclaredData, uc *models.UserConfig, citizenFound, selfDeclaredFound bool, contaOrigem string) *clients.SalesforceCidadaoCreateRequest {
	patch := buildSalesforceSnapshotPatch(citizen, sd, uc, citizenFound, selfDeclaredFound)
	fields := patch.Fields()

	req := &clients.SalesforceCidadaoCreateRequest{
		ContaOrigem: contaOrigem,
	}
	if citizen != nil {
		req.CPF = citizen.CPF
	}
	if sd != nil && req.CPF == "" {
		req.CPF = sd.CPF
	}

	req.Nome = stringField(fields, "nome")
	req.NomeSocial = stringField(fields, "nomeSocial")
	req.NomeExibicao = stringField(fields, "nomeExibicao")
	req.Email = stringField(fields, "email")
	req.Telefone1 = stringField(fields, "telefone1")
	req.Telefone2 = stringField(fields, "telefone2")
	req.Telefone3 = stringField(fields, "telefone3")
	req.TipoTelefone1 = stringField(fields, "tipoTelefone1")
	req.TipoTelefone2 = stringField(fields, "tipoTelefone2")
	req.TipoTelefone3 = stringField(fields, "tipoTelefone3")
	req.TelefoneInternacional = stringField(fields, "telefoneInternacional")
	req.Genero = stringField(fields, "genero")
	req.Raca = stringField(fields, "raca")
	req.Escolaridade = stringField(fields, "escolaridade")
	req.RendaFamiliar = stringField(fields, "rendaFamiliar")
	req.Deficiencia = stringField(fields, "deficiencia")
	req.DataNascimento = stringField(fields, "dataNascimento")
	req.Complemento = stringField(fields, "complemento")
	req.Nacionalidade = stringField(fields, "nacionalidade")
	req.Passaporte = stringField(fields, "passaporte")
	req.Cidade = stringField(fields, "cidade")
	req.CanalOrigem = stringField(fields, "canalOrigem")
	req.CanalUltimaModificacao = stringField(fields, "canalUltimaModificacao")
	if idioma, ok := fields["idioma"].([]string); ok && len(idioma) > 0 {
		req.Idioma = idioma
	}
	if tourist, ok := fields["isTourist"].(bool); ok {
		req.IsTourist = &tourist
	}
	if addr, ok := fields["endereco"].(*clients.SalesforceEndereco); ok && addr != nil {
		req.Endereco = addr
	}
	return req
}

type rmiStringValue struct {
	set   bool
	clear bool
	value string
}

func putString(patch *clients.SalesforceCidadaoPatchRequest, key string, v rmiStringValue) {
	if !v.set {
		return
	}
	if v.clear {
		patch.PutClear(key)
		return
	}
	patch.Put(key, v.value)
}

func putIdioma(patch *clients.SalesforceCidadaoPatchRequest, sd *models.SelfDeclaredData, selfDeclaredFound bool) {
	if !selfDeclaredFound || sd == nil || sd.Idioma == nil {
		return
	}
	if len(sd.Idioma) == 0 {
		patch.PutClear("idioma")
		return
	}
	patch.Put("idioma", append([]string(nil), sd.Idioma...))
}

func putIsTourist(patch *clients.SalesforceCidadaoPatchRequest, sd *models.SelfDeclaredData, selfDeclaredFound bool) {
	if !selfDeclaredFound || sd == nil || sd.IsTourist == nil {
		return
	}
	patch.Put("isTourist", *sd.IsTourist)
}

func putConsentimento(patch *clients.SalesforceCidadaoPatchRequest, uc *models.UserConfig) {
	if uc == nil || len(uc.SalesforceConsentimentos) == 0 {
		return
	}
	items := make([]clients.SalesforceConsentimento, 0, len(uc.SalesforceConsentimentos))
	for _, entry := range uc.SalesforceConsentimentos {
		item := clients.SalesforceConsentimento{
			Categoria:  entry.Categoria,
			Status:     entry.Status,
			Canal:      entry.Canal,
			Data:       entry.Data,
			Finalidade: entry.Finalidade,
			Motivo:     entry.Motivo,
		}
		if item.Status == "" {
			if entry.OptIn {
				item.Acao = "optin"
			} else {
				item.Acao = "optout"
			}
		}
		items = append(items, item)
	}
	if len(items) > 0 {
		patch.Put("consentimento", items)
	}
}

func putEndereco(patch *clients.SalesforceCidadaoPatchRequest, sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound, citizenFound bool) {
	addr, hasAddr := rmiEndereco(sd, citizen, selfDeclaredFound, citizenFound)
	if !hasAddr {
		return
	}
	if addr == nil {
		patch.PutClear("endereco")
		return
	}
	patch.PutEndereco(addr)
}

func rmiNome(citizen *models.Citizen) rmiStringValue {
	if citizen == nil || citizen.Nome == nil {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: strings.TrimSpace(*citizen.Nome)}
}

func rmiNomeSocial(citizen *models.Citizen) rmiStringValue {
	if citizen == nil || citizen.NomeSocial == nil {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: strings.TrimSpace(*citizen.NomeSocial)}
}

func rmiPrimeiroNome(citizen *models.Citizen) rmiStringValue {
	nome := rmiNome(citizen)
	if nome.clear {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: firstName(nome.value)}
}

func rmiSelfDeclaredString(sd *models.SelfDeclaredData, pick func(*models.SelfDeclaredData) *string) rmiStringValue {
	if sd == nil {
		return rmiStringValue{set: true, clear: true}
	}
	val := pick(sd)
	if val == nil {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: strings.TrimSpace(*val)}
}

func rmiSelfDeclaredComplemento(sd *models.SelfDeclaredData) rmiStringValue {
	if sd == nil {
		return rmiStringValue{set: true, clear: true}
	}
	if sd.Complemento != nil {
		return rmiStringValue{set: true, value: strings.TrimSpace(*sd.Complemento)}
	}
	if sd.Endereco != nil && sd.Endereco.Principal != nil && sd.Endereco.Principal.Complemento != nil {
		return rmiStringValue{set: true, value: strings.TrimSpace(*sd.Endereco.Principal.Complemento)}
	}
	return rmiStringValue{set: true, clear: true}
}

func rmiEmail(sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound bool) rmiStringValue {
	if selfDeclaredFound && sd != nil && sd.Email != nil {
		if sd.Email.Principal == nil || sd.Email.Principal.Valor == nil {
			return rmiStringValue{set: true, clear: true}
		}
		return rmiStringValue{set: true, value: strings.TrimSpace(*sd.Email.Principal.Valor)}
	}
	if citizen != nil && citizen.Email != nil && citizen.Email.Principal != nil && citizen.Email.Principal.Valor != nil {
		return rmiStringValue{set: true, value: strings.TrimSpace(*citizen.Email.Principal.Valor)}
	}
	if selfDeclaredFound {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{}
}

func rmiTelefonePrincipal(sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound bool) rmiStringValue {
	if selfDeclaredFound && sd != nil && sd.Telefone != nil {
		if sd.Telefone.Principal == nil || sd.Telefone.Principal.Valor == nil || strings.TrimSpace(*sd.Telefone.Principal.Valor) == "" {
			return rmiStringValue{set: true, clear: true}
		}
	}
	phone := extractPhoneDigits(sd, citizen)
	if phone == "" {
		if selfDeclaredFound {
			return rmiStringValue{set: true, clear: true}
		}
		return rmiStringValue{}
	}
	return rmiStringValue{set: true, value: clients.NormalizeSalesforcePhone(phone)}
}

func rmiTelefoneAlternativo(sd *models.SelfDeclaredData, index int) rmiStringValue {
	if sd == nil || sd.Telefone == nil || index >= len(sd.Telefone.Alternativo) {
		return rmiStringValue{set: true, clear: true}
	}
	alt := sd.Telefone.Alternativo[index]
	if alt.Valor == nil || strings.TrimSpace(*alt.Valor) == "" {
		return rmiStringValue{set: true, clear: true}
	}
	phone := formatTelefoneAlternativo(alt)
	if phone == "" {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: clients.NormalizeSalesforcePhone(phone)}
}

func formatTelefoneAlternativo(alt models.TelefoneAlternativo) string {
	if alt.DDI != nil && alt.DDD != nil && alt.Valor != nil {
		return utils.FormatPhoneForStorage(*alt.DDI, *alt.DDD, *alt.Valor)
	}
	if alt.Valor != nil {
		return strings.TrimSpace(*alt.Valor)
	}
	return ""
}

func rmiGenero(sd *models.SelfDeclaredData) rmiStringValue {
	val := rmiSelfDeclaredString(sd, func(s *models.SelfDeclaredData) *string { return s.Genero })
	if val.clear {
		return val
	}
	mapped := mapRMIGeneroToSalesforce(val.value)
	if mapped == "" {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: mapped}
}

func rmiRaca(sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound, citizenFound bool) rmiStringValue {
	if selfDeclaredFound && sd != nil && sd.Raca != nil {
		mapped := mapRMIRacaToSalesforce(*sd.Raca)
		if mapped == "" {
			return rmiStringValue{set: true, clear: true}
		}
		return rmiStringValue{set: true, value: mapped}
	}
	if citizenFound && citizen != nil && citizen.Raca != nil {
		mapped := mapRMIRacaToSalesforce(*citizen.Raca)
		if mapped == "" {
			return rmiStringValue{set: true, clear: true}
		}
		return rmiStringValue{set: true, value: mapped}
	}
	if selfDeclaredFound || citizenFound {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{}
}

func rmiDataNascimento(citizen *models.Citizen) rmiStringValue {
	if citizen == nil || citizen.Nascimento == nil || citizen.Nascimento.Data == nil {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{set: true, value: citizen.Nascimento.Data.Format("2006-01-02")}
}

func rmiCidade(sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound, citizenFound bool) rmiStringValue {
	if selfDeclaredFound && sd != nil && sd.Endereco != nil && sd.Endereco.Principal != nil && sd.Endereco.Principal.Municipio != nil {
		return rmiStringValue{set: true, value: strings.TrimSpace(*sd.Endereco.Principal.Municipio)}
	}
	if citizenFound && citizen != nil && citizen.Endereco != nil && citizen.Endereco.Principal != nil && citizen.Endereco.Principal.Municipio != nil {
		return rmiStringValue{set: true, value: strings.TrimSpace(*citizen.Endereco.Principal.Municipio)}
	}
	if selfDeclaredFound || citizenFound {
		return rmiStringValue{set: true, clear: true}
	}
	return rmiStringValue{}
}

func rmiEndereco(sd *models.SelfDeclaredData, citizen *models.Citizen, selfDeclaredFound, citizenFound bool) (*clients.SalesforceEndereco, bool) {
	var principal *models.EnderecoPrincipal
	fromSelfDeclared := false
	if selfDeclaredFound && sd != nil && sd.Endereco != nil && sd.Endereco.Principal != nil {
		p := *sd.Endereco.Principal
		principal = &p
		fromSelfDeclared = true
	} else if citizenFound && citizen != nil && citizen.Endereco != nil && citizen.Endereco.Principal != nil {
		p := *citizen.Endereco.Principal
		principal = &p
	}
	if principal == nil {
		if selfDeclaredFound || citizenFound {
			return nil, true
		}
		return nil, false
	}

	addr := &clients.SalesforceEndereco{
		Logradouro:  deref(principal.Logradouro),
		Cidade:      deref(principal.Municipio),
		Estado:      deref(principal.Estado),
		CEP:         deref(principal.CEP),
		Complemento: deref(principal.Complemento),
		Bairro:      deref(principal.Bairro),
	}
	if fromSelfDeclared && addr.Logradouro == "" && addr.Cidade == "" && addr.Estado == "" && addr.CEP == "" && addr.Complemento == "" && addr.Bairro == "" {
		return nil, true
	}
	if addr.Logradouro == "" && addr.Cidade == "" && addr.Estado == "" && addr.CEP == "" && addr.Complemento == "" && addr.Bairro == "" {
		if selfDeclaredFound {
			return nil, true
		}
		return nil, false
	}
	if addr.Cidade != "" || addr.Estado != "" || addr.CEP != "" {
		addr.Pais = "Brasil"
	}
	return addr, true
}

func stringField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	raw, ok := fields[key]
	if !ok || raw == nil {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return s
}
