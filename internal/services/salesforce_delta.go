package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func parseSalesforceUpdatedAt(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t, nil
		} else {
			lastErr = err
		}
	}
	return nil, fmt.Errorf("invalid updatedAt: %w", lastErr)
}

func isStaleSalesforceUpdatedAt(existing *time.Time, incoming *time.Time) bool {
	if incoming == nil || existing == nil {
		return false
	}
	return !incoming.After(*existing)
}

func (w *SyncWorker) applySalesforceDelta(ctx context.Context, cpf, evento, updatedAtRaw string, rawDados json.RawMessage, now time.Time) error {
	incomingUpdatedAt, err := parseSalesforceUpdatedAt(updatedAtRaw)
	if err != nil {
		return err
	}

	var existing models.SelfDeclaredData
	findErr := w.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(ctx, bson.M{"cpf": cpf}).Decode(&existing)
	existingFound := findErr == nil
	if findErr != nil && !errors.Is(findErr, mongo.ErrNoDocuments) {
		return fmt.Errorf("failed to load self_declared for delta: %w", findErr)
	}

	if existingFound && isStaleSalesforceUpdatedAt(existing.SalesforceUpdatedAt, incomingUpdatedAt) {
		w.logger.Info("salesforce inbound sync skipped: stale updatedAt",
			zap.String("cpf", cpf),
			zap.String("evento", evento))
		return nil
	}

	fields, err := parseSalesforceDeltaFields(rawDados)
	if err != nil {
		return err
	}

	var existingPtr *models.SelfDeclaredData
	if existingFound {
		existingPtr = &existing
	}

	set, unset := buildSelfDeclaredDeltaPatch(existingPtr, fields, evento, now, incomingUpdatedAt)
	if !salesforceDeltaHasWork(fields, set, unset) {
		w.logger.Info("salesforce inbound sync: empty delta",
			zap.String("cpf", cpf))
		return nil
	}

	set["cpf"] = cpf
	set["updated_at"] = now

	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}

	coll := w.mongo.Collection(config.AppConfig.SelfDeclaredCollection)
	if _, err := coll.UpdateOne(ctx, bson.M{"cpf": cpf}, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("failed to apply salesforce delta to self_declared: %w", err)
	}

	if fieldPresent(fields, "consentimento") {
		if err := w.applySalesforceConsentimentoDelta(ctx, cpf, fields["consentimento"], now); err != nil {
			return fmt.Errorf("failed to apply salesforce consentimento delta: %w", err)
		}
	}

	// Stamp the idempotency watermark only after every mutation succeeded.
	// Writing it earlier makes a later failure look stale on retry (partial apply, no DLQ).
	if err := w.stampSalesforceUpdatedAt(ctx, cpf, incomingUpdatedAt, now); err != nil {
		return fmt.Errorf("failed to stamp salesforce updatedAt: %w", err)
	}

	w.invalidateSalesforceMirrorCaches(ctx, cpf)
	return nil
}

func parseSalesforceDeltaFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("dados is empty")
	}
	if jsonRawIsNull(raw) {
		return nil, fmt.Errorf("dados cannot be null")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("dados must be a JSON object: %w", err)
	}
	return fields, nil
}

func fieldPresent(fields map[string]json.RawMessage, key string) bool {
	_, ok := fields[key]
	return ok
}

func buildSelfDeclaredDeltaPatch(existing *models.SelfDeclaredData, fields map[string]json.RawMessage, evento string, now time.Time, incomingUpdatedAt *time.Time) (bson.M, bson.M) {
	set := bson.M{}
	unset := bson.M{}

	origem := salesforceOrigem
	sistema := salesforceSistema
	set["salesforce_synced_at"] = now
	_ = incomingUpdatedAt // watermark is committed after all mutations succeed

	if evento == SalesforceWebhookEventAnonimizacao {
		set["salesforce_anonymized"] = true
		set["salesforce_anonymized_at"] = now
	}

	if raw, ok := fields["accountId"]; ok {
		if jsonRawIsNull(raw) {
			unset["salesforce_account_id"] = ""
		} else if val, ok := jsonRawAsString(raw); ok {
			set["salesforce_account_id"] = val
		}
	}

	if raw, ok := fields["email"]; ok {
		applyEmailDelta(set, unset, raw, now, origem, sistema)
	}

	if raw, ok := fields["telefone1"]; ok {
		applyPhoneDelta(set, unset, raw, now, origem, sistema)
	} else if raw, ok := fields["telefonePrincipal"]; ok {
		applyPhoneDelta(set, unset, raw, now, origem, sistema)
	}

	if raw, ok := fields["endereco"]; ok {
		applyEnderecoDelta(set, unset, existing, raw, now, origem, sistema)
	}

	if raw, ok := fields["cidade"]; ok && !fieldPresent(fields, "endereco") {
		wrapped, err := json.Marshal(map[string]json.RawMessage{"cidade": raw})
		if err == nil {
			applyEnderecoDelta(set, unset, existing, wrapped, now, origem, sistema)
		}
	}

	if raw, ok := fields["raca"]; ok {
		applyStringPointerDelta(set, unset, "raca", raw, mapSalesforceRacaToRMI)
	}

	if raw, ok := fields["genero"]; ok {
		applyStringPointerDelta(set, unset, "genero", raw, mapSalesforceGeneroToRMI)
	}

	if raw, ok := fields["nomeExibicao"]; ok {
		applyPlainStringPointerDelta(set, unset, "nome_exibicao", raw)
	}

	if raw, ok := fields["escolaridade"]; ok {
		applyPlainStringPointerDelta(set, unset, "escolaridade", raw)
	}
	if raw, ok := fields["rendaFamiliar"]; ok {
		applyPlainStringPointerDelta(set, unset, "renda_familiar", raw)
	}
	if raw, ok := fields["deficiencia"]; ok {
		applyPlainStringPointerDelta(set, unset, "deficiencia", raw)
	}

	if raw, ok := fields["nacionalidade"]; ok {
		applyPlainStringPointerDelta(set, unset, "nacionalidade", raw)
	}
	if raw, ok := fields["passaporte"]; ok {
		applyPlainStringPointerDelta(set, unset, "passaporte", raw)
	}
	if raw, ok := fields["complemento"]; ok {
		applyPlainStringPointerDelta(set, unset, "complemento", raw)
	}
	if raw, ok := fields["telefoneInternacional"]; ok {
		applyPlainStringPointerDelta(set, unset, "telefone_internacional", raw)
	}
	if raw, ok := fields["tipoTelefone1"]; ok {
		applyPlainStringPointerDelta(set, unset, "tipo_telefone1", raw)
	}
	if raw, ok := fields["tipoTelefone2"]; ok {
		applyPlainStringPointerDelta(set, unset, "tipo_telefone2", raw)
	}
	if raw, ok := fields["tipoTelefone3"]; ok {
		applyPlainStringPointerDelta(set, unset, "tipo_telefone3", raw)
	}
	if raw, ok := fields["canalOrigem"]; ok {
		applyPlainStringPointerDelta(set, unset, "canal_origem", raw)
	}
	if raw, ok := fields["canalUltimaModificacao"]; ok {
		applyPlainStringPointerDelta(set, unset, "canal_ultima_modificacao", raw)
	}
	if raw, ok := fields["idioma"]; ok {
		applyStringSliceDelta(set, unset, "idioma", raw)
	}
	if raw, ok := fields["isTourist"]; ok {
		applyBoolPointerDelta(set, unset, "is_tourist", raw)
	}

	if raw, ok := fields["telefone2"]; ok {
		applyAlternatePhoneDelta(set, unset, existing, raw, 0, now, origem, sistema)
	}
	if raw, ok := fields["telefone3"]; ok {
		applyAlternatePhoneDelta(set, unset, existing, raw, 1, now, origem, sistema)
	}

	return set, unset
}

func salesforceDeltaHasWork(fields map[string]json.RawMessage, set, unset bson.M) bool {
	if fieldPresent(fields, "consentimento") {
		return true
	}
	for k := range set {
		if k != "salesforce_synced_at" && k != "salesforce_updated_at" && k != "salesforce_anonymized" && k != "salesforce_anonymized_at" {
			return true
		}
	}
	return len(unset) > 0
}

func (w *SyncWorker) stampSalesforceUpdatedAt(ctx context.Context, cpf string, incoming *time.Time, now time.Time) error {
	if incoming == nil || w == nil || w.mongo == nil || config.AppConfig == nil {
		return nil
	}
	_, err := w.mongo.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(ctx,
		bson.M{"cpf": cpf},
		bson.M{"$set": bson.M{
			"salesforce_updated_at": *incoming,
			"salesforce_synced_at":  now,
		}},
	)
	return err
}

func applyStringSliceDelta(set, unset bson.M, fieldKey string, raw json.RawMessage) {
	if jsonRawIsNull(raw) {
		unset[fieldKey] = ""
		return
	}
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return
	}
	set[fieldKey] = items
}

func applyBoolPointerDelta(set, unset bson.M, fieldKey string, raw json.RawMessage) {
	if jsonRawIsNull(raw) {
		unset[fieldKey] = ""
		return
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return
	}
	set[fieldKey] = &b
}

func applyAlternatePhoneDelta(set, unset bson.M, existing *models.SelfDeclaredData, raw json.RawMessage, index int, now time.Time, origem, sistema string) {
	if jsonRawIsNull(raw) {
		// Clear alternate slot by rebuilding telefone without that alternativo index.
		tel := cloneTelefone(existing)
		if tel == nil {
			return
		}
		if index < len(tel.Alternativo) {
			tel.Alternativo = append(tel.Alternativo[:index], tel.Alternativo[index+1:]...)
		}
		if len(tel.Alternativo) == 0 && tel.Principal == nil {
			unset["telefone"] = ""
			return
		}
		set["telefone"] = tel
		return
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return
	}
	tel := cloneTelefone(existing)
	if tel == nil {
		tel = &models.Telefone{Indicador: utils.BoolPtr(true)}
	}
	for len(tel.Alternativo) <= index {
		tel.Alternativo = append(tel.Alternativo, models.TelefoneAlternativo{})
	}
	alt := buildTelefoneAlternativo(val, origem, sistema)
	tel.Alternativo[index] = alt
	set["telefone"] = tel
}

func cloneTelefone(existing *models.SelfDeclaredData) *models.Telefone {
	if existing == nil || existing.Telefone == nil {
		return nil
	}
	t := *existing.Telefone
	if existing.Telefone.Principal != nil {
		p := *existing.Telefone.Principal
		t.Principal = &p
	}
	if len(existing.Telefone.Alternativo) > 0 {
		t.Alternativo = append([]models.TelefoneAlternativo(nil), existing.Telefone.Alternativo...)
	}
	return &t
}

func buildTelefoneAlternativo(phone, origem, sistema string) models.TelefoneAlternativo {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		empty := ""
		return models.TelefoneAlternativo{DDI: &empty, DDD: &empty, Valor: &empty, Origem: &origem, Sistema: &sistema}
	}
	components, err := utils.ParsePhoneNumber(phone)
	if err != nil {
		empty := ""
		val := phone
		return models.TelefoneAlternativo{Valor: &val, Origem: &origem, Sistema: &sistema, DDI: &empty, DDD: &empty}
	}
	ddi, ddd, valor := components.DDI, components.DDD, components.Valor
	return models.TelefoneAlternativo{DDI: &ddi, DDD: &ddd, Valor: &valor, Origem: &origem, Sistema: &sistema}
}

func salesforceConsentimentoKey(categoria, canal string) string {
	categoria = strings.TrimSpace(categoria)
	canal = strings.TrimSpace(canal)
	if canal == "" {
		return categoria
	}
	return categoria + "|" + canal
}

func isUnsafeMongoMapKey(key string) bool {
	return strings.ContainsAny(key, ".$")
}

func applyPlainStringPointerDelta(set, unset bson.M, fieldKey string, raw json.RawMessage) {
	if jsonRawIsNull(raw) {
		unset[fieldKey] = ""
		return
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return
	}
	set[fieldKey] = &val
}

func applyStringPointerDelta(set, unset bson.M, fieldKey string, raw json.RawMessage, transform func(string) string) {
	if jsonRawIsNull(raw) {
		unset[fieldKey] = ""
		return
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return
	}
	mapped := transform(val)
	set[fieldKey] = &mapped
}

func applyEmailDelta(set, unset bson.M, raw json.RawMessage, now time.Time, origem, sistema string) {
	if jsonRawIsNull(raw) {
		unset["email"] = ""
		return
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return
	}
	set["email"] = &models.Email{
		Indicador: utils.BoolPtr(true),
		Principal: &models.EmailPrincipal{
			Valor:     &val,
			Origem:    &origem,
			Sistema:   &sistema,
			UpdatedAt: &now,
		},
	}
}

func applyPhoneDelta(set, unset bson.M, raw json.RawMessage, now time.Time, origem, sistema string) {
	if jsonRawIsNull(raw) {
		unset["telefone"] = ""
		return
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return
	}
	if strings.TrimSpace(val) == "" {
		empty := ""
		set["telefone"] = &models.Telefone{
			Indicador: utils.BoolPtr(true),
			Principal: &models.TelefonePrincipal{
				DDI:       &empty,
				DDD:       &empty,
				Valor:     &empty,
				Origem:    &origem,
				Sistema:   &sistema,
				UpdatedAt: &now,
			},
		}
		return
	}
	if tel := buildSalesforceTelefoneField(val, now); tel != nil {
		set["telefone"] = tel
	}
}

func applyEnderecoDelta(set, unset bson.M, existing *models.SelfDeclaredData, raw json.RawMessage, now time.Time, origem, sistema string) {
	if jsonRawIsNull(raw) {
		unset["endereco"] = ""
		return
	}
	var addrFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &addrFields); err != nil {
		return
	}

	logradouro, hasLog := enderecoSubfield(addrFields, "logradouro")
	cidade, hasCid := enderecoSubfield(addrFields, "cidade")
	estado, hasEst := enderecoSubfield(addrFields, "estado")
	cep, hasCEP := enderecoSubfield(addrFields, "cep")
	complemento, hasComp := enderecoSubfield(addrFields, "complemento")
	bairro, hasBairro := enderecoSubfield(addrFields, "bairro")

	if !hasLog && !hasCid && !hasEst && !hasCEP && !hasComp && !hasBairro {
		return
	}

	var base *models.EnderecoPrincipal
	if existing != nil && existing.Endereco != nil && existing.Endereco.Principal != nil {
		p := *existing.Endereco.Principal
		base = &p
	}

	l := mergeEnderecoPart(logradouro, hasLog, base, func(p *models.EnderecoPrincipal) *string { return p.Logradouro })
	c := mergeEnderecoPart(cidade, hasCid, base, func(p *models.EnderecoPrincipal) *string { return p.Municipio })
	e := mergeEnderecoPart(estado, hasEst, base, func(p *models.EnderecoPrincipal) *string { return p.Estado })
	cepVal := mergeEnderecoPart(cep, hasCEP, base, func(p *models.EnderecoPrincipal) *string { return p.CEP })
	comp := mergeEnderecoPart(complemento, hasComp, base, func(p *models.EnderecoPrincipal) *string { return p.Complemento })
	bairroVal := mergeEnderecoPart(bairro, hasBairro, base, func(p *models.EnderecoPrincipal) *string { return p.Bairro })

	set["endereco"] = &models.Endereco{
		Indicador: utils.BoolPtr(true),
		Principal: &models.EnderecoPrincipal{
			Logradouro:  &l,
			Municipio:   &c,
			Estado:      &e,
			CEP:         &cepVal,
			Complemento: &comp,
			Bairro:      &bairroVal,
			Origem:      &origem,
			Sistema:     &sistema,
			UpdatedAt:   &now,
		},
	}
}

func mergeEnderecoPart(delta *string, hasDelta bool, base *models.EnderecoPrincipal, pick func(*models.EnderecoPrincipal) *string) string {
	if hasDelta {
		if delta != nil {
			return *delta
		}
		return ""
	}
	if base != nil {
		if v := pick(base); v != nil {
			return *v
		}
	}
	return ""
}

func enderecoSubfield(addrFields map[string]json.RawMessage, key string) (*string, bool) {
	raw, ok := addrFields[key]
	if !ok {
		return nil, false
	}
	if jsonRawIsNull(raw) {
		empty := ""
		return &empty, true
	}
	val, ok := jsonRawAsString(raw)
	if !ok {
		return nil, true
	}
	return &val, true
}

func (w *SyncWorker) applySalesforceConsentimentoDelta(ctx context.Context, cpf string, raw json.RawMessage, now time.Time) error {
	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.UserConfigCollection) == "" {
		return nil
	}

	coll := w.mongo.Collection(config.AppConfig.UserConfigCollection)

	if jsonRawIsNull(raw) {
		_, err := coll.UpdateOne(ctx,
			bson.M{"cpf": cpf},
			bson.M{
				"$set": bson.M{
					"salesforce_consentimentos": map[string]models.SalesforceConsentimentoEntry{},
					"updated_at":                now,
				},
				"$setOnInsert": bson.M{
					"cpf":         cpf,
					"first_login": false,
				},
			},
			options.Update().SetUpsert(true),
		)
		if err != nil {
			return err
		}
		_ = w.redis.Del(ctx, fmt.Sprintf("user_config:%s", cpf)).Err()
		return nil
	}

	var items []clients.SalesforceConsentimento
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("invalid consentimento delta: %w", err)
	}

	set := bson.M{
		"updated_at": now,
	}
	for _, item := range items {
		categoria := strings.TrimSpace(item.Categoria)
		if categoria == "" {
			continue
		}
		canal := strings.TrimSpace(item.Canal)
		key := salesforceConsentimentoKey(categoria, canal)
		if isUnsafeMongoMapKey(key) {
			w.logger.Warn("skipping salesforce consentimento key with unsafe mongo path characters",
				zap.String("cpf", cpf),
				zap.String("key", key))
			continue
		}
		optInVal := salesforceConsentimentoIsOptIn(item)
		set["salesforce_consentimentos."+key] = models.SalesforceConsentimentoEntry{
			Categoria:  categoria,
			Status:     strings.TrimSpace(item.Status),
			Canal:      canal,
			Data:       strings.TrimSpace(item.Data),
			Finalidade: strings.TrimSpace(item.Finalidade),
			Motivo:     strings.TrimSpace(item.Motivo),
			OptIn:      optInVal,
		}
	}

	_, err := coll.UpdateOne(ctx,
		bson.M{"cpf": cpf},
		bson.M{
			"$set": set,
			"$setOnInsert": bson.M{
				"cpf":         cpf,
				"first_login": false,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return err
	}
	_ = w.redis.Del(ctx, fmt.Sprintf("user_config:%s", cpf)).Err()
	return nil
}

func jsonRawIsNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func jsonRawAsString(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func mapSalesforceConsentimentoToOptIn(items []clients.SalesforceConsentimento) (bool, map[string]bool) {
	categoryOptIns := make(map[string]bool)
	anyIn := false
	for _, item := range items {
		categoria := strings.TrimSpace(item.Categoria)
		if categoria == "" {
			continue
		}
		key := salesforceConsentimentoKey(categoria, item.Canal)
		optIn := salesforceConsentimentoIsOptIn(item)
		categoryOptIns[key] = optIn
		if optIn {
			anyIn = true
		}
	}
	return anyIn, categoryOptIns
}

func salesforceConsentimentoIsOptIn(item clients.SalesforceConsentimento) bool {
	switch strings.ToUpper(strings.TrimSpace(item.Status)) {
	case "IN":
		return true
	case "OUT":
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Acao)) {
	case "optin":
		return true
	case "optout":
		return false
	}
	return false
}

// mapSalesforceCidadaoToSelfDeclared builds a partial $set map from a cidadao struct (test/helper).
func mapSalesforceCidadaoToSelfDeclared(c *clients.SalesforceCidadao) bson.M {
	if c == nil {
		return bson.M{}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return bson.M{}
	}
	set, _ := buildSelfDeclaredDeltaPatch(nil, jsonFields(raw), SalesforceWebhookEventAtualizacao, time.Now(), nil)
	return set
}

func jsonFields(raw []byte) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return map[string]json.RawMessage{}
	}
	return fields
}
