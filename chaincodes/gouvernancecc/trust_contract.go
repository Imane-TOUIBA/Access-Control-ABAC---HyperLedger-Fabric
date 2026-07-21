package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// TrustContract gère le registre des conventions de collaboration
// inter-organisationnelles, déployé sur global-channel.
//
// Sémantique d'une convention : si OwnerOrg enregistre une convention avec
// PartnerOrg pour le projet ProjectID, cela signifie que OwnerOrg autorise
// PartnerOrg à accéder à SES PROPRES ressources (celles dont OwnerOrg est
// propriétaire) dans le cadre de ce projet. La convention est donc toujours
// déclarée par l'organisation qui POSSÈDE la ressource, jamais par celle
// qui la demande.
//
// Ceci implémente conv_valide(org1, org2, proj, h) du formalisme :
// conv_valide(owner(o), owner(u), proj, h) doit être vrai pour que Ptrust
// soit satisfaite lors d'une demande d'accès.
type TrustContract struct {
	contractapi.Contract
}

// Convention représente un accord de collaboration unidirectionnel.
type Convention struct {
	OwnerOrg     string `json:"owner_org"`     // organisation qui accorde l'accès à ses ressources
	PartnerOrg   string `json:"partner_org"`   // organisation bénéficiaire de l'accès
	ProjectID    string `json:"project_id"`    // projet couvert par la convention
	ExpiresAt    string `json:"expires_at"`    // date d'expiration, format RFC3339
	Revoked      bool   `json:"revoked"`       // révocation explicite, indépendante de l'expiration
	RevokedAt    string `json:"revoked_at"`    // horodatage de la révocation, vide si non révoquée
	RegisteredBy string `json:"registered_by"` // MSP ID ayant soumis l'enregistrement (audit)
}

// convKey construit la clé composite d'une convention.
// Format : CONV~{OwnerOrg}~{PartnerOrg}~{ProjectID}
// Ce choix de clé permet une lecture en O(1) par GetState lors de
// l'évaluation de Ptrust, sans avoir à itérer sur une liste de projets.
func convKey(ctx contractapi.TransactionContextInterface, ownerOrg, partnerOrg, projectID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("CONV", []string{ownerOrg, partnerOrg, projectID})
}

// RegisterConvention enregistre une nouvelle convention de collaboration.
// Seule l'organisation propriétaire (OwnerOrg) peut enregistrer une
// convention en son propre nom, car c'est elle qui accorde l'accès à ses
// ressources. Cette restriction est vérifiée via l'identité MSP du
// soumetteur de la transaction.
func (tc *TrustContract) RegisterConvention(
	ctx contractapi.TransactionContextInterface,
	ownerOrg string,
	partnerOrg string,
	projectID string,
	expiresAt string,
) error {

	// -- Vérification de l'autorité d'écriture --
	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RegisterConvention : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != ownerOrg {
		return fmt.Errorf(
			"RegisterConvention : seule l'organisation propriétaire %s peut enregistrer cette convention, appel reçu de %s",
			ownerOrg, callerMSP,
		)
	}

	if ownerOrg == "" || partnerOrg == "" || projectID == "" {
		return fmt.Errorf("RegisterConvention : owner_org, partner_org et project_id sont obligatoires")
	}
	if ownerOrg == partnerOrg {
		return fmt.Errorf("RegisterConvention : une organisation ne peut pas déclarer une convention avec elle-même")
	}

	// -- Validation de la date d'expiration --
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return fmt.Errorf("RegisterConvention : expires_at invalide, format attendu RFC3339 : %w", err)
	}
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("RegisterConvention : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	now := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))
	if !now.Before(expiry) {
		return fmt.Errorf("RegisterConvention : la date d'expiration doit être dans le futur")
	}

	conv := Convention{
		OwnerOrg:     ownerOrg,
		PartnerOrg:   partnerOrg,
		ProjectID:    projectID,
		ExpiresAt:    expiresAt,
		Revoked:      false,
		RevokedAt:    "",
		RegisteredBy: callerMSP,
	}

	key, err := convKey(ctx, ownerOrg, partnerOrg, projectID)
	if err != nil {
		return fmt.Errorf("RegisterConvention : erreur de construction de clé : %w", err)
	}

	data, err := json.Marshal(conv)
	if err != nil {
		return fmt.Errorf("RegisterConvention : erreur de sérialisation : %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return fmt.Errorf("RegisterConvention : erreur d'écriture sur le ledger : %w", err)
	}

	return ctx.GetStub().SetEvent("ConventionRegistered", data)
}

// RevokeConvention révoque explicitement une convention existante.
// Seule l'organisation propriétaire peut révoquer sa propre convention.
// La révocation est une sentinelle indépendante de la date d'expiration :
// une convention révoquée est invalide même si elle n'est pas encore expirée.
func (tc *TrustContract) RevokeConvention(
	ctx contractapi.TransactionContextInterface,
	ownerOrg string,
	partnerOrg string,
	projectID string,
) error {

	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RevokeConvention : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != ownerOrg {
		return fmt.Errorf(
			"RevokeConvention : seule l'organisation propriétaire %s peut révoquer cette convention, appel reçu de %s",
			ownerOrg, callerMSP,
		)
	}

	key, err := convKey(ctx, ownerOrg, partnerOrg, projectID)
	if err != nil {
		return fmt.Errorf("RevokeConvention : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("RevokeConvention : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return fmt.Errorf("RevokeConvention : aucune convention trouvée pour %s -> %s sur le projet %s", ownerOrg, partnerOrg, projectID)
	}

	var conv Convention
	if err := json.Unmarshal(data, &conv); err != nil {
		return fmt.Errorf("RevokeConvention : erreur de désérialisation : %w", err)
	}

	if conv.Revoked {
		return fmt.Errorf("RevokeConvention : cette convention est déjà révoquée")
	}

	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("RevokeConvention : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	now := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))

	conv.Revoked = true
	conv.RevokedAt = now.UTC().Format(time.RFC3339)

	updated, err := json.Marshal(conv)
	if err != nil {
		return fmt.Errorf("RevokeConvention : erreur de sérialisation : %w", err)
	}

	if err := ctx.GetStub().PutState(key, updated); err != nil {
		return fmt.Errorf("RevokeConvention : erreur d'écriture sur le ledger : %w", err)
	}

	return ctx.GetStub().SetEvent("ConventionRevoked", updated)
}

// GetConvention est une fonction de requête publique, utile pour
// l'interface graphique et pour le débogage manuel pendant les tests.
func (tc *TrustContract) GetConvention(
	ctx contractapi.TransactionContextInterface,
	ownerOrg string,
	partnerOrg string,
	projectID string,
) (*Convention, error) {

	key, err := convKey(ctx, ownerOrg, partnerOrg, projectID)
	if err != nil {
		return nil, fmt.Errorf("GetConvention : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetConvention : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetConvention : aucune convention trouvée pour %s -> %s sur le projet %s", ownerOrg, partnerOrg, projectID)
	}

	var conv Convention
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("GetConvention : erreur de désérialisation : %w", err)
	}

	return &conv, nil
}

// IsConventionValid implémente conv_valide(ownerOrg, partnerOrg, projectID, h).
// Cette fonction est exportée (majuscule) pour être appelée directement
// (appel Go interne, pas InvokeChaincode) depuis AttestationContract, puisque
// les deux contrats partagent le même binaire et le même world state au sein
// de global-channel. Elle n'est volontairement pas exposée comme transaction
// invocable séparément côté SDK (elle n'a pas besoin de l'être : c'est une
// fonction de lecture utilitaire interne au chaincode).
//
// Retourne (true, nil) si la convention existe, n'est pas révoquée, et n'est
// pas expirée à l'instant h. Retourne (false, nil) dans tous les autres cas
// métier (absence, révocation, expiration), et (false, err) seulement en cas
// d'erreur technique (lecture ledger, désérialisation).
func IsConventionValid(
	ctx contractapi.TransactionContextInterface,
	ownerOrg string,
	partnerOrg string,
	projectID string,
	h time.Time,
) (bool, error) {

	key, err := convKey(ctx, ownerOrg, partnerOrg, projectID)
	if err != nil {
		return false, fmt.Errorf("IsConventionValid : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return false, fmt.Errorf("IsConventionValid : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		// Pas de convention enregistrée : Ptrust échoue, ce n'est pas une erreur technique.
		return false, nil
	}

	var conv Convention
	if err := json.Unmarshal(data, &conv); err != nil {
		return false, fmt.Errorf("IsConventionValid : erreur de désérialisation : %w", err)
	}

	if conv.Revoked {
		return false, nil
	}

	expiry, err := time.Parse(time.RFC3339, conv.ExpiresAt)
	if err != nil {
		return false, fmt.Errorf("IsConventionValid : date d'expiration stockée invalide : %w", err)
	}

	return h.Before(expiry), nil
}
