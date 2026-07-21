package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// AttestationContract est déployé sur global-channel. Il gère :
//   - le registre des ressources (ResourceDefinition)
//   - le registre des consentements patients (ConsentRecord), révocables
//   - le point d'entrée principal SubmitAttestation, qui orchestre
//     Prequester (déclaratif, déjà vérifié localement par le PEP) puis
//     Ptrust (appel direct à TrustContract.IsConventionValid, même binaire)
//     puis le consentement individuel si la ressource le requiert.
//
// Le résultat est stocké sur le ledger de global-channel ET émis comme
// événement Fabric ("AttestationValidated"). Cet événement est ce que le
// service relais (Node.js, identité CGNMSP ou IBMSP) écoute pour
// transmettre la demande validée à PolicyContract sur project-channel,
// car un appel direct en écriture entre deux canaux différents n'est pas
// possible nativement dans Hyperledger Fabric.
type AttestationContract struct {
	contractapi.Contract
}

// ---------------------------------------------------------------------------
// ResourceDefinition
// ---------------------------------------------------------------------------

// ResourceDefinition porte uniquement ce qui est nécessaire à l'évaluation
// de Ptrust et du consentement sur global-channel. Les attributs nécessaires
// à Powner (organisations autorisées, actions permises, habilitation minimale,
// plage horaire) sont volontairement absents ici : ils sont définis dans
// ResourcePolicy, sur project-channel, car Powner y est évalué.
type ResourceDefinition struct {
	ResourceID      string `json:"resource_id"`
	OwnerOrg        string `json:"owner_org"`
	ConsentRequired bool   `json:"consent_required"`
	ProjectID       string `json:"project_id"`
}

func resourceKey(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("RES", []string{resourceID})
}

// RegisterResource enregistre une nouvelle ressource. Seule l'organisation
// propriétaire déclarée (ownerOrg) peut enregistrer une ressource en son
// propre nom.
func (ac *AttestationContract) RegisterResource(
	ctx contractapi.TransactionContextInterface,
	resourceID string,
	ownerOrg string,
	consentRequired bool,
	projectID string,
) error {

	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RegisterResource : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != ownerOrg {
		return fmt.Errorf(
			"RegisterResource : seule l'organisation propriétaire %s peut enregistrer cette ressource, appel reçu de %s",
			ownerOrg, callerMSP,
		)
	}
	if resourceID == "" || ownerOrg == "" || projectID == "" {
		return fmt.Errorf("RegisterResource : resource_id, owner_org et project_id sont obligatoires")
	}

	res := ResourceDefinition{
		ResourceID:      resourceID,
		OwnerOrg:        ownerOrg,
		ConsentRequired: consentRequired,
		ProjectID:       projectID,
	}

	key, err := resourceKey(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("RegisterResource : erreur de construction de clé : %w", err)
	}

	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("RegisterResource : erreur de sérialisation : %w", err)
	}

	return ctx.GetStub().PutState(key, data)
}

// GetResource retourne la définition d'une ressource.
func (ac *AttestationContract) GetResource(
	ctx contractapi.TransactionContextInterface,
	resourceID string,
) (*ResourceDefinition, error) {

	key, err := resourceKey(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("GetResource : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetResource : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetResource : ressource %s introuvable", resourceID)
	}

	var res ResourceDefinition
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("GetResource : erreur de désérialisation : %w", err)
	}

	return &res, nil
}

// ---------------------------------------------------------------------------
// ConsentRecord (mode individuel uniquement pour cette version du prototype)
// ---------------------------------------------------------------------------

// ConsentRecord représente le consentement d'un patient précis pour qu'une
// organisation donnée accède à une ressource donnée dans le cadre d'un
// projet donné. Le mode agrégé (PAT_aut, calcul sur un ensemble de patients)
// n'est pas implémenté dans cette première itération du prototype.
type ConsentRecord struct {
	PatientID  string `json:"patient_id"`
	OrgID      string `json:"org_id"`
	ResourceID string `json:"resource_id"`
	ProjectID  string `json:"project_id"`
	Status     string `json:"status"` // "actif" ou "revoque"
	ExpiresAt  string `json:"expires_at"`
	RevokedAt  string `json:"revoked_at"`
}

func consentKey(ctx contractapi.TransactionContextInterface, patientID, orgID, resourceID, projectID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("CONSENT", []string{patientID, orgID, resourceID, projectID})
}

// RegisterConsent enregistre un consentement patient. Seule l'organisation
// propriétaire de la ressource concernée peut enregistrer ce consentement,
// car c'est elle qui est responsable de la collecte du consentement pour
// ses propres ressources (cohérent avec le rôle de HU dans le scénario
// complet : seule l'organisation détentrice de la donnée gère le
// consentement associé).
func (ac *AttestationContract) RegisterConsent(
	ctx contractapi.TransactionContextInterface,
	patientID string,
	orgID string,
	resourceID string,
	projectID string,
	expiresAt string,
) error {

	res, err := ac.GetResource(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("RegisterConsent : %w", err)
	}

	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RegisterConsent : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != res.OwnerOrg {
		return fmt.Errorf(
			"RegisterConsent : seule l'organisation propriétaire %s de la ressource %s peut enregistrer ce consentement, appel reçu de %s",
			res.OwnerOrg, resourceID, callerMSP,
		)
	}

	if patientID == "" || orgID == "" {
		return fmt.Errorf("RegisterConsent : patient_id et org_id sont obligatoires")
	}

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return fmt.Errorf("RegisterConsent : expires_at invalide, format attendu RFC3339 : %w", err)
	}
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("RegisterConsent : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	now := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))
	if !now.Before(expiry) {
		return fmt.Errorf("RegisterConsent : la date d'expiration doit être dans le futur")
	}

	rec := ConsentRecord{
		PatientID:  patientID,
		OrgID:      orgID,
		ResourceID: resourceID,
		ProjectID:  projectID,
		Status:     "actif",
		ExpiresAt:  expiresAt,
		RevokedAt:  "",
	}

	key, err := consentKey(ctx, patientID, orgID, resourceID, projectID)
	if err != nil {
		return fmt.Errorf("RegisterConsent : erreur de construction de clé : %w", err)
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("RegisterConsent : erreur de sérialisation : %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return fmt.Errorf("RegisterConsent : erreur d'écriture sur le ledger : %w", err)
	}

	return ctx.GetStub().SetEvent("ConsentRegistered", data)
}

// RevokeConsent révoque un consentement existant. Seule l'organisation
// propriétaire de la ressource concernée peut le révoquer.
func (ac *AttestationContract) RevokeConsent(
	ctx contractapi.TransactionContextInterface,
	patientID string,
	orgID string,
	resourceID string,
	projectID string,
) error {

	res, err := ac.GetResource(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("RevokeConsent : %w", err)
	}

	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RevokeConsent : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != res.OwnerOrg {
		return fmt.Errorf(
			"RevokeConsent : seule l'organisation propriétaire %s de la ressource %s peut révoquer ce consentement, appel reçu de %s",
			res.OwnerOrg, resourceID, callerMSP,
		)
	}

	key, err := consentKey(ctx, patientID, orgID, resourceID, projectID)
	if err != nil {
		return fmt.Errorf("RevokeConsent : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("RevokeConsent : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return fmt.Errorf("RevokeConsent : aucun consentement trouvé pour ce patient sur cette ressource")
	}

	var rec ConsentRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("RevokeConsent : erreur de désérialisation : %w", err)
	}

	if rec.Status == "revoque" {
		return fmt.Errorf("RevokeConsent : ce consentement est déjà révoqué")
	}

	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return fmt.Errorf("RevokeConsent : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	now := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))

	rec.Status = "revoque"
	rec.RevokedAt = now.UTC().Format(time.RFC3339)

	updated, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("RevokeConsent : erreur de sérialisation : %w", err)
	}

	if err := ctx.GetStub().PutState(key, updated); err != nil {
		return fmt.Errorf("RevokeConsent : erreur d'écriture sur le ledger : %w", err)
	}

	return ctx.GetStub().SetEvent("ConsentRevoked", updated)
}

// GetConsent retourne un enregistrement de consentement, utile pour
// l'interface graphique et le débogage.
func (ac *AttestationContract) GetConsent(
	ctx contractapi.TransactionContextInterface,
	patientID string,
	orgID string,
	resourceID string,
	projectID string,
) (*ConsentRecord, error) {

	key, err := consentKey(ctx, patientID, orgID, resourceID, projectID)
	if err != nil {
		return nil, fmt.Errorf("GetConsent : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetConsent : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetConsent : aucun consentement trouvé")
	}

	var rec ConsentRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("GetConsent : erreur de désérialisation : %w", err)
	}

	return &rec, nil
}

// isConsentValid implémente cv(patientID, orgID, resourceID, projectID, h)
// du formalisme, en mode individuel uniquement.
func isConsentValid(
	ctx contractapi.TransactionContextInterface,
	patientID string,
	orgID string,
	resourceID string,
	projectID string,
	h time.Time,
) (bool, error) {

	key, err := consentKey(ctx, patientID, orgID, resourceID, projectID)
	if err != nil {
		return false, fmt.Errorf("isConsentValid : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return false, fmt.Errorf("isConsentValid : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return false, nil
	}

	var rec ConsentRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return false, fmt.Errorf("isConsentValid : erreur de désérialisation : %w", err)
	}

	if rec.Status != "actif" {
		return false, nil
	}

	expiry, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err != nil {
		return false, fmt.Errorf("isConsentValid : date d'expiration stockée invalide : %w", err)
	}

	return h.Before(expiry), nil
}

// ---------------------------------------------------------------------------
// RequesterAttestation et AttestationResult
// ---------------------------------------------------------------------------

// RequesterAttestation est le document soumis par le PEP de l'organisation
// requérante après évaluation locale de Prequester. Le périmètre du stage
// considère l'authentification et la signature cryptographique de ce
// document comme déjà résolues en amont (hors scope) : ce chaincode vérifie
// uniquement le résultat déclaré (PrequesterOK) et s'appuie sur l'identité
// MSP Fabric du soumetteur de la transaction pour garantir que l'attestation
// provient bien de RequesterOrg.
type RequesterAttestation struct {
	RequesterOrg  string `json:"requester_org"`
	UserID        string `json:"user_id"`
	UserClearance string `json:"user_clearance"` // "standard" ou "elevee", utilisé plus tard par Powner
	ResourceID    string `json:"resource_id"`
	Action        string `json:"action"`
	ProjectID     string `json:"project_id"`
	PatientID     string `json:"patient_id"` // vide si la ressource ne requiert pas de consentement
	PrequesterOK  bool   `json:"prequester_ok"`
	Nonce         string `json:"nonce"` // anti-rejeu
}

// AttestationResult est le résultat complet de l'orchestration
// Prequester -> Ptrust -> Consentement. Il est stocké sur le ledger de
// global-channel et émis comme événement Fabric pour être relayé vers
// PolicyContract sur project-channel.
type AttestationResult struct {
	AttestationID string `json:"attestation_id"`
	RequesterOrg  string `json:"requester_org"`
	UserID        string `json:"user_id"`
	UserClearance string `json:"user_clearance"`
	ResourceID    string `json:"resource_id"`
	OwnerOrg      string `json:"owner_org"`
	Action        string `json:"action"`
	ProjectID     string `json:"project_id"`
	PatientID     string `json:"patient_id"`

	PrequesterOK    bool `json:"prequester_ok"`
	PtrustOK        bool `json:"ptrust_ok"`
	ConsentRequired bool `json:"consent_required"`
	ConsentValid    bool `json:"consent_valid"`

	Valid      bool   `json:"valid"`
	DenyReason string `json:"deny_reason"`
	Timestamp  string `json:"timestamp"`
}

func attestationKey(ctx contractapi.TransactionContextInterface, attestationID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("ATTEST", []string{attestationID})
}

// usedNonceKey construit la clé de vérification anti-rejeu.
func usedNonceKey(ctx contractapi.TransactionContextInterface, requesterOrg, nonce string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("NONCE", []string{requesterOrg, nonce})
}

// SubmitAttestation est le point d'entrée principal du flux de contrôle
// d'accès sur global-channel. Il orchestre, dans l'ordre fail-fast :
//
//  1. Prequester : vérification déclarative (le PEP a déjà fait le travail
//     localement ; le chaincode vérifie seulement la cohérence du champ
//     PrequesterOK et l'identité MSP du soumetteur).
//  2. Ptrust : appel direct (Go, même binaire) à IsConventionValid, qui
//     vérifie qu'une convention valide et non révoquée existe entre
//     l'organisation propriétaire de la ressource et l'organisation
//     requérante, pour le projet demandé.
//  3. Consentement : si la ressource le requiert, vérifie le consentement
//     individuel du patient désigné.
//
// Le résultat est stocké sur le ledger ET émis comme événement
// "AttestationValidated", que le service relais transmettra à
// PolicyContract sur project-channel si Valid == true.
func (ac *AttestationContract) SubmitAttestation(
	ctx contractapi.TransactionContextInterface,
	attestationJSON string,
) (*AttestationResult, error) {

	var attest RequesterAttestation
	if err := json.Unmarshal([]byte(attestationJSON), &attest); err != nil {
		return nil, fmt.Errorf("SubmitAttestation : attestation JSON invalide : %w", err)
	}

	// -- Vérification que le soumetteur de la transaction appartient bien
	// à l'organisation déclarée comme requérante --
	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != attest.RequesterOrg {
		return nil, fmt.Errorf(
			"SubmitAttestation : l'attestation déclare requester_org=%s mais la transaction est soumise par %s",
			attest.RequesterOrg, callerMSP,
		)
	}

	// -- Anti-rejeu --
	if attest.Nonce == "" {
		return nil, fmt.Errorf("SubmitAttestation : un nonce est obligatoire")
	}
	nKey, err := usedNonceKey(ctx, attest.RequesterOrg, attest.Nonce)
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : erreur de construction de clé nonce : %w", err)
	}
	existingNonce, err := ctx.GetStub().GetState(nKey)
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : erreur de lecture du ledger pour le nonce : %w", err)
	}
	if existingNonce != nil {
		return nil, fmt.Errorf("SubmitAttestation : ce nonce a déjà été utilisé, rejeu détecté")
	}
	if err := ctx.GetStub().PutState(nKey, []byte("used")); err != nil {
		return nil, fmt.Errorf("SubmitAttestation : erreur d'écriture du nonce : %w", err)
	}

	// -- Horodatage déterministe --
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	h := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))
	txID := ctx.GetStub().GetTxID()

	attestationID := fmt.Sprintf("%x", sha256.Sum256([]byte(txID+attest.ResourceID+attest.UserID)))

	result := AttestationResult{
		AttestationID: attestationID,
		RequesterOrg:  attest.RequesterOrg,
		UserID:        attest.UserID,
		UserClearance: attest.UserClearance,
		ResourceID:    attest.ResourceID,
		Action:        attest.Action,
		ProjectID:     attest.ProjectID,
		PatientID:     attest.PatientID,
		PrequesterOK:  attest.PrequesterOK,
		Timestamp:     h.UTC().Format(time.RFC3339),
		Valid:         false,
	}

	// -- Étape 1 : Prequester --
	if !attest.PrequesterOK {
		result.DenyReason = "PREQUESTER_FAIL : l'attestation locale déclare un refus Prequester"
		return ac.storeAndEmit(ctx, result)
	}

	// -- Résolution de la ressource --
	res, err := ac.GetResource(ctx, attest.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : %w", err)
	}
	result.OwnerOrg = res.OwnerOrg
	result.ConsentRequired = res.ConsentRequired

	if attest.ProjectID != res.ProjectID {
		result.DenyReason = fmt.Sprintf(
			"PREQUESTER_FAIL : projet demandé %s ne correspond pas au projet de la ressource %s",
			attest.ProjectID, res.ProjectID,
		)
		return ac.storeAndEmit(ctx, result)
	}

	// -- Étape 2 : Ptrust --
	// Convention attendue : OwnerOrg (propriétaire de la ressource) autorise
	// RequesterOrg (demandeur) pour ProjectID.
	ptrustOK, err := IsConventionValid(ctx, res.OwnerOrg, attest.RequesterOrg, attest.ProjectID, h)
	if err != nil {
		return nil, fmt.Errorf("SubmitAttestation : erreur lors de l'évaluation de Ptrust : %w", err)
	}
	result.PtrustOK = ptrustOK
	if !ptrustOK {
		result.DenyReason = fmt.Sprintf(
			"PTRUST_FAIL : aucune convention valide et non révoquée de %s vers %s pour le projet %s",
			res.OwnerOrg, attest.RequesterOrg, attest.ProjectID,
		)
		return ac.storeAndEmit(ctx, result)
	}

	// -- Étape 3 : Consentement (mode individuel uniquement) --
	if res.ConsentRequired {
		if attest.PatientID == "" {
			result.DenyReason = "CONSENT_FAIL : la ressource requiert un consentement mais aucun patient_id n'a été fourni"
			return ac.storeAndEmit(ctx, result)
		}
		consentOK, err := isConsentValid(ctx, attest.PatientID, attest.RequesterOrg, attest.ResourceID, attest.ProjectID, h)
		if err != nil {
			return nil, fmt.Errorf("SubmitAttestation : erreur lors de l'évaluation du consentement : %w", err)
		}
		result.ConsentValid = consentOK
		if !consentOK {
			result.DenyReason = fmt.Sprintf(
				"CONSENT_FAIL : consentement absent, expiré ou révoqué pour le patient %s",
				attest.PatientID,
			)
			return ac.storeAndEmit(ctx, result)
		}
	} else {
		result.ConsentValid = true
	}

	// -- Toutes les conditions évaluées sur global-channel sont satisfaites --
	result.Valid = true
	return ac.storeAndEmit(ctx, result)
}

// storeAndEmit sérialise le résultat, le stocke sur le ledger, émet
// l'événement Fabric "AttestationValidated" (que le résultat soit positif
// ou négatif, pour garantir une traçabilité complète et auditable de
// chaque tentative sur global-channel), puis le retourne à l'appelant.
func (ac *AttestationContract) storeAndEmit(
	ctx contractapi.TransactionContextInterface,
	result AttestationResult,
) (*AttestationResult, error) {

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("storeAndEmit : erreur de sérialisation : %w", err)
	}

	key, err := attestationKey(ctx, result.AttestationID)
	if err != nil {
		return nil, fmt.Errorf("storeAndEmit : erreur de construction de clé : %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return nil, fmt.Errorf("storeAndEmit : erreur d'écriture sur le ledger : %w", err)
	}

	if err := ctx.GetStub().SetEvent("AttestationValidated", data); err != nil {
		return nil, fmt.Errorf("storeAndEmit : erreur d'émission de l'événement : %w", err)
	}

	return &result, nil
}

// GetAttestationResult retourne un résultat d'attestation stocké, utile
// pour le débogage et l'audit manuel.
func (ac *AttestationContract) GetAttestationResult(
	ctx contractapi.TransactionContextInterface,
	attestationID string,
) (*AttestationResult, error) {

	key, err := attestationKey(ctx, attestationID)
	if err != nil {
		return nil, fmt.Errorf("GetAttestationResult : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetAttestationResult : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetAttestationResult : aucun résultat trouvé pour %s", attestationID)
	}

	var result AttestationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("GetAttestationResult : erreur de désérialisation : %w", err)
	}

	return &result, nil
}
