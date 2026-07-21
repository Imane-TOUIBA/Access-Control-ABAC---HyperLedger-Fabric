package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// PolicyContract est déployé sur project-channel (Org1MSP, Org2MSP).
// Il assure trois responsabilités distinctes mais liées :
//
//  1. RegisterResourcePolicy : permet à l'organisation propriétaire d'une
//     ressource d'enregistrer la politique d'accès associée (Powner).
//  2. EvaluatePolicy : reçoit le résultat d'attestation transmis par le
//     service relais depuis global-channel, évalue Powner dans l'ordre
//     fail-fast, et enregistre la décision finale.
//  3. GetDecision : permet de consulter une décision enregistrée (audit).
type PolicyContract struct {
	contractapi.Contract
}

// ---------------------------------------------------------------------------
// ResourcePolicy
// ---------------------------------------------------------------------------

// ResourcePolicy porte tous les attributs nécessaires à l'évaluation de
// Powner. Elle est définie indépendamment de ResourceDefinition (qui vit
// sur global-channel), car project-channel ne peut pas lire en direct
// l'état de global-channel : chaque canal doit porter ce qui lui est
// nécessaire pour évaluer sa portion de la politique de manière autonome.
type ResourcePolicy struct {
	ResourceID        string   `json:"resource_id"`
	OwnerOrg          string   `json:"owner_org"`
	AuthorizedOrgs    []string `json:"authorized_orgs"`     // organisations autorisées à demander un accès
	AllowedActions    []string `json:"allowed_actions"`     // actions permises sur cette ressource
	MinClearance      string   `json:"min_clearance"`       // "standard" ou "elevee"
	AccessWindowStart string   `json:"access_window_start"` // format "HH:MM", heure UTC
	AccessWindowEnd   string   `json:"access_window_end"`   // format "HH:MM", heure UTC
}

// clearanceRank établit l'ordre total entre les niveaux d'habilitation.
// Un niveau plus élevé satisfait toujours une exigence de niveau plus bas.
var clearanceRank = map[string]int{
	"standard": 1,
	"elevee":   2,
}

func resourcePolicyKey(ctx contractapi.TransactionContextInterface, resourceID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("RESPOLICY", []string{resourceID})
}

// RegisterResourcePolicy enregistre la politique d'accès d'une ressource.
// Seule l'organisation propriétaire déclarée peut l'enregistrer.
func (pc *PolicyContract) RegisterResourcePolicy(
	ctx contractapi.TransactionContextInterface,
	resourceID string,
	ownerOrg string,
	authorizedOrgsJSON string,
	allowedActionsJSON string,
	minClearance string,
	accessWindowStart string,
	accessWindowEnd string,
) error {

	callerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("RegisterResourcePolicy : impossible de résoudre l'identité MSP appelante : %w", err)
	}
	if callerMSP != ownerOrg {
		return fmt.Errorf(
			"RegisterResourcePolicy : seule l'organisation propriétaire %s peut enregistrer cette politique, appel reçu de %s",
			ownerOrg, callerMSP,
		)
	}

	if resourceID == "" || ownerOrg == "" {
		return fmt.Errorf("RegisterResourcePolicy : resource_id et owner_org sont obligatoires")
	}

	if _, ok := clearanceRank[minClearance]; !ok {
		return fmt.Errorf("RegisterResourcePolicy : min_clearance invalide, attendu 'standard' ou 'elevee', reçu '%s'", minClearance)
	}

	var authorizedOrgs []string
	if err := json.Unmarshal([]byte(authorizedOrgsJSON), &authorizedOrgs); err != nil {
		return fmt.Errorf("RegisterResourcePolicy : authorized_orgs JSON invalide : %w", err)
	}

	var allowedActions []string
	if err := json.Unmarshal([]byte(allowedActionsJSON), &allowedActions); err != nil {
		return fmt.Errorf("RegisterResourcePolicy : allowed_actions JSON invalide : %w", err)
	}

	if err := validateTimeFormat(accessWindowStart); err != nil {
		return fmt.Errorf("RegisterResourcePolicy : access_window_start invalide : %w", err)
	}
	if err := validateTimeFormat(accessWindowEnd); err != nil {
		return fmt.Errorf("RegisterResourcePolicy : access_window_end invalide : %w", err)
	}

	policy := ResourcePolicy{
		ResourceID:        resourceID,
		OwnerOrg:          ownerOrg,
		AuthorizedOrgs:    authorizedOrgs,
		AllowedActions:    allowedActions,
		MinClearance:      minClearance,
		AccessWindowStart: accessWindowStart,
		AccessWindowEnd:   accessWindowEnd,
	}

	key, err := resourcePolicyKey(ctx, resourceID)
	if err != nil {
		return fmt.Errorf("RegisterResourcePolicy : erreur de construction de clé : %w", err)
	}

	data, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("RegisterResourcePolicy : erreur de sérialisation : %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return fmt.Errorf("RegisterResourcePolicy : erreur d'écriture sur le ledger : %w", err)
	}

	return ctx.GetStub().SetEvent("ResourcePolicyRegistered", data)
}

// GetResourcePolicy retourne la politique enregistrée pour une ressource.
func (pc *PolicyContract) GetResourcePolicy(
	ctx contractapi.TransactionContextInterface,
	resourceID string,
) (*ResourcePolicy, error) {

	key, err := resourcePolicyKey(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("GetResourcePolicy : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetResourcePolicy : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetResourcePolicy : aucune politique trouvée pour la ressource %s", resourceID)
	}

	var policy ResourcePolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("GetResourcePolicy : erreur de désérialisation : %w", err)
	}

	return &policy, nil
}

// validateTimeFormat vérifie qu'une chaîne respecte le format "HH:MM".
func validateTimeFormat(t string) error {
	_, err := time.Parse("15:04", t)
	return err
}

// ---------------------------------------------------------------------------
// AttestationResult (structure miroir, reçue du relais depuis global-channel)
// ---------------------------------------------------------------------------

// AttestationResultInput est la structure que le service relais transmet
// à EvaluatePolicy. Elle reprend exactement les champs émis par
// l'événement "AttestationValidated" d'AttestationContract sur
// global-channel. La duplication de structure entre les deux chaincodes
// est volontaire : project-channel ne peut pas importer le type Go
// d'un autre chaincode déployé sur un canal différent, et surtout,
// chaque canal doit rester autonome et ne fait confiance qu'à ce qui
// lui est explicitement soumis comme transaction, pas à un état partagé.
type AttestationResultInput struct {
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

// ---------------------------------------------------------------------------
// DecisionRecord
// ---------------------------------------------------------------------------

// DecisionRecord est la trace d'audit finale, qui combine les résultats de
// global-channel (Prequester, Ptrust, consentement) avec ceux évalués
// localement sur project-channel (Powner), pour produire une décision
// PERMIT/DENY complète et explicable.
type DecisionRecord struct {
	DecisionID   string `json:"decision_id"` // = AttestationID
	RequesterOrg string `json:"requester_org"`
	UserID       string `json:"user_id"`
	OwnerOrg     string `json:"owner_org"`
	ResourceID   string `json:"resource_id"`
	Action       string `json:"action"`
	ProjectID    string `json:"project_id"`
	PatientID    string `json:"patient_id"`

	// Résultats repris de global-channel, pour une décision auditable
	// de manière autonome sur project-channel sans avoir à recroiser
	// les deux ledgers.
	PrequesterOK bool `json:"prequester_ok"`
	PtrustOK     bool `json:"ptrust_ok"`
	ConsentValid bool `json:"consent_valid"`

	// Résultats des 4 conditions de Powner, évaluées dans l'ordre fail-fast
	OrgAuthorized       bool `json:"org_authorized"`
	ActionAllowed       bool `json:"action_allowed"`
	ClearanceSufficient bool `json:"clearance_sufficient"`
	WithinWindow        bool `json:"within_window"`
	PownerOK            bool `json:"powner_ok"`

	Decision   string `json:"decision"` // "PERMIT" ou "DENY"
	DenyReason string `json:"deny_reason"`
	Timestamp  string `json:"timestamp"`
}

func decisionKey(ctx contractapi.TransactionContextInterface, decisionID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("DECISION", []string{decisionID})
}

// EvaluatePolicy est le point d'entrée appelé par le service relais.
// Il reçoit le résultat de l'attestation validée sur global-channel,
// vérifie qu'elle était effectivement positive (Valid == true côté
// global-channel, sinon il n'y a rien à évaluer côté Powner), résout
// la politique de la ressource, évalue les 4 conditions de Powner dans
// l'ordre fail-fast, puis enregistre la décision finale.
//
// Le relais doit soumettre cette transaction avec une identité MSP membre
// de project-channel (Org1MSP ou Org2MSP) ; Org3MSP ne peut pas soumettre
// cette transaction puisqu'elle n'est pas membre de ce canal.
func (pc *PolicyContract) EvaluatePolicy(
	ctx contractapi.TransactionContextInterface,
	attestationResultJSON string,
) (*DecisionRecord, error) {

	var attestResult AttestationResultInput
	if err := json.Unmarshal([]byte(attestationResultJSON), &attestResult); err != nil {
		return nil, fmt.Errorf("EvaluatePolicy : résultat d'attestation JSON invalide : %w", err)
	}

	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, fmt.Errorf("EvaluatePolicy : impossible d'obtenir l'horodatage de transaction : %w", err)
	}
	h := time.Unix(txTimestamp.GetSeconds(), int64(txTimestamp.GetNanos()))

	decision := DecisionRecord{
		DecisionID:   attestResult.AttestationID,
		RequesterOrg: attestResult.RequesterOrg,
		UserID:       attestResult.UserID,
		OwnerOrg:     attestResult.OwnerOrg,
		ResourceID:   attestResult.ResourceID,
		Action:       attestResult.Action,
		ProjectID:    attestResult.ProjectID,
		PatientID:    attestResult.PatientID,
		PrequesterOK: attestResult.PrequesterOK,
		PtrustOK:     attestResult.PtrustOK,
		ConsentValid: attestResult.ConsentValid,
		Timestamp:    h.UTC().Format(time.RFC3339),
	}

	// -- Garde-fou : si l'attestation n'était pas valide sur global-channel,
	// Powner ne doit pas être évalué (fail-fast déjà appliqué en amont).
	if !attestResult.Valid {
		decision.Decision = "DENY"
		decision.DenyReason = "ATTESTATION_INVALID : " + attestResult.DenyReason
		return pc.storeDecision(ctx, decision)
	}

	// -- Résolution de la politique de ressource --
	policy, err := pc.GetResourcePolicy(ctx, attestResult.ResourceID)
	if err != nil {
		decision.Decision = "DENY"
		decision.DenyReason = fmt.Sprintf("POWNER_FAIL : %v", err)
		return pc.storeDecision(ctx, decision)
	}

	// -- Condition 1 : organisation autorisée --
	decision.OrgAuthorized = contains(policy.AuthorizedOrgs, attestResult.RequesterOrg)
	if !decision.OrgAuthorized {
		decision.Decision = "DENY"
		decision.DenyReason = fmt.Sprintf(
			"POWNER_FAIL : organisation %s non autorisée sur la ressource %s",
			attestResult.RequesterOrg, attestResult.ResourceID,
		)
		return pc.storeDecision(ctx, decision)
	}

	// -- Condition 2 : action autorisée --
	decision.ActionAllowed = contains(policy.AllowedActions, attestResult.Action)
	if !decision.ActionAllowed {
		decision.Decision = "DENY"
		decision.DenyReason = fmt.Sprintf(
			"POWNER_FAIL : action %s non autorisée sur la ressource %s",
			attestResult.Action, attestResult.ResourceID,
		)
		return pc.storeDecision(ctx, decision)
	}

	// -- Condition 3 : habilitation suffisante --
	decision.ClearanceSufficient = clearanceRank[attestResult.UserClearance] >= clearanceRank[policy.MinClearance]
	if !decision.ClearanceSufficient {
		decision.Decision = "DENY"
		decision.DenyReason = fmt.Sprintf(
			"POWNER_FAIL : habilitation insuffisante (%s requis au minimum, %s fournie)",
			policy.MinClearance, attestResult.UserClearance,
		)
		return pc.storeDecision(ctx, decision)
	}

	// -- Condition 4 : plage horaire valide --
	decision.WithinWindow = inAccessWindow(h, policy.AccessWindowStart, policy.AccessWindowEnd)
	if !decision.WithinWindow {
		decision.Decision = "DENY"
		decision.DenyReason = fmt.Sprintf(
			"POWNER_FAIL : accès hors de la plage horaire autorisée [%s - %s] UTC",
			policy.AccessWindowStart, policy.AccessWindowEnd,
		)
		return pc.storeDecision(ctx, decision)
	}

	// -- Toutes les conditions de Powner sont satisfaites --
	decision.PownerOK = true
	decision.Decision = "PERMIT"
	return pc.storeDecision(ctx, decision)
}

// contains vérifie la présence d'une valeur dans une liste de chaînes.
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// inAccessWindow vérifie que l'heure UTC de h est comprise entre start et
// end, au format "HH:MM".
func inAccessWindow(h time.Time, start, end string) bool {
	current := fmt.Sprintf("%02d:%02d", h.UTC().Hour(), h.UTC().Minute())
	return current >= start && current <= end
}

// storeDecision persiste la décision finale sur le ledger de
// project-channel, émet un événement d'audit, et retourne la décision.
func (pc *PolicyContract) storeDecision(
	ctx contractapi.TransactionContextInterface,
	decision DecisionRecord,
) (*DecisionRecord, error) {

	data, err := json.Marshal(decision)
	if err != nil {
		return nil, fmt.Errorf("storeDecision : erreur de sérialisation : %w", err)
	}

	key, err := decisionKey(ctx, decision.DecisionID)
	if err != nil {
		return nil, fmt.Errorf("storeDecision : erreur de construction de clé : %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return nil, fmt.Errorf("storeDecision : erreur d'écriture sur le ledger : %w", err)
	}

	if err := ctx.GetStub().SetEvent("DecisionRecorded", data); err != nil {
		return nil, fmt.Errorf("storeDecision : erreur d'émission de l'événement : %w", err)
	}

	return &decision, nil
}

// GetDecision retourne une décision enregistrée, pour l'audit et
// l'interface graphique.
func (pc *PolicyContract) GetDecision(
	ctx contractapi.TransactionContextInterface,
	decisionID string,
) (*DecisionRecord, error) {

	key, err := decisionKey(ctx, decisionID)
	if err != nil {
		return nil, fmt.Errorf("GetDecision : erreur de construction de clé : %w", err)
	}

	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("GetDecision : erreur de lecture du ledger : %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("GetDecision : aucune décision trouvée pour %s", decisionID)
	}

	var decision DecisionRecord
	if err := json.Unmarshal(data, &decision); err != nil {
		return nil, fmt.Errorf("GetDecision : erreur de désérialisation : %w", err)
	}

	return &decision, nil
}
