package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Academia struct {
	BaseAggregate
	
	Type           string
	Nome           string
	CodigoAcademia string
	SenhaHash      string
	Provincia      string
	Endereco       string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Status         string // inativo, ativo
	Cursos         []string
	CreatedAt      time.Time
	
	TotalEstudantes          int
	TotalInscricoesPendentes int
}

func NewAcademia() *Academia {
	log.Printf("[ACADEMIA_AGGREGATE] Criando nova instância de Academia")
	return &Academia{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status: "inativo",
		Cursos: []string{},
	}
}

func (a *Academia) GetType() string {
	return "Academia"
}

func (a *Academia) Apply(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando evento: type=%s, aggregate_id=%s", 
		event.GetEventType(), event.GetAggregateID())
	
	switch event.GetEventType() {
	case "AcademiaCriada":
		return a.applyAcademiaCriada(event)
	case "InscricaoAprovada":
		return a.applyInscricaoAprovada(event)
	case "InscricaoReprovada":
		return a.applyInscricaoReprovada(event)
	case "AcademiaAtivada":
		return a.applyAcademiaAtivada(event)
	case "AcademiaDesativada":
		return a.applyAcademiaDesativada(event)
	case "CursosAtualizados":
		return a.applyCursosAtualizados(event)
	case "AcademiaDadosAtualizados":
		return a.applyAcademiaDadosAtualizados(event)
	default:
		log.Printf("[ACADEMIA_AGGREGATE] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (a *Academia) Criar(
	tipo string,
	nome string,
	codigoAcademia string,
	senhaHash string,
	provincia string,
	endereco string,
	numeroTelefone *string,
	email *string,
	website *string,
	nivelEscolar *string,
	cursos []string,
) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando Criar: nome=%s, codigo=%s, tipo=%s", 
		nome, codigoAcademia, tipo)
	
	if tipo != "escola" && tipo != "superior" {
		log.Printf("[ACADEMIA_AGGREGATE] Tipo inválido: %s", tipo)
		return fmt.Errorf("tipo deve ser 'escola' ou 'superior'")
	}
	if nome == "" {
		log.Printf("[ACADEMIA_AGGREGATE] Nome obrigatório não fornecido")
		return fmt.Errorf("nome é obrigatório")
	}
	if codigoAcademia == "" {
		log.Printf("[ACADEMIA_AGGREGATE] Código obrigatório não fornecido")
		return fmt.Errorf("código é obrigatório")
	}
	
	if tipo == "escola" {
		if nivelEscolar == nil {
			log.Printf("[ACADEMIA_AGGREGATE] Nível escolar obrigatório para escolas")
			return fmt.Errorf("nivel_escolar é obrigatório para escolas")
		}
		
		validNiveis := map[string]bool{
			"fundamental": true,
			"medio":       true,
			"misto":       true,
		}
		
		if !validNiveis[*nivelEscolar] {
			log.Printf("[ACADEMIA_AGGREGATE] Nível escolar inválido: %s", *nivelEscolar)
			return fmt.Errorf("nivel_escolar deve ser 'fundamental', 'medio' ou 'misto'")
		}
	}

	event := &AcademiaCriadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaCriada",
			AggregateID: a.ID,
		},
		Type:           tipo,
		Nome:           nome,
		CodigoAcademia: codigoAcademia,
		SenhaHash:      senhaHash,
		Provincia:      provincia,
		Endereco:       endereco,
		NumeroTelefone: numeroTelefone,
		Email:          email,
		Website:        website,
		NivelEscolar:   nivelEscolar,
		Cursos:         cursos,
		CreatedAt:      time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento AcademiaCriada gerado para academia: %s", a.ID)
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AprovarInscricao(
	estudanteID uuid.UUID,
	inscricaoID uuid.UUID,
	tipo string,
	anoInscricao string,
	curso *string,
) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando AprovarInscricao: estudante=%s, inscricao=%s", 
		estudanteID, inscricaoID)
	
	if a.Status != "ativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia inativa, não pode aprovar inscrições: status=%s", a.Status)
		return fmt.Errorf("academia está inativa - não pode aprovar inscrições")
	}

	event := &InscricaoAprovadaPorAcademiaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoAprovada",
			AggregateID: a.ID,
		},
		EstudanteID:  estudanteID,
		InscricaoID:  inscricaoID,
		AcademiaID:   a.ID,
		Tipo:         tipo,
		AnoInscricao: anoInscricao,
		Curso:        curso,
		ApprovedAt:   time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento InscricaoAprovada gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) ReprovarInscricao(
	estudanteID uuid.UUID,
	inscricaoID uuid.UUID,
	motivo string,
) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando ReprovarInscricao: estudante=%s, inscricao=%s, motivo=%s", 
		estudanteID, inscricaoID, motivo)
	
	if a.Status != "ativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia inativa, não pode reprovar inscrições: status=%s", a.Status)
		return fmt.Errorf("academia está inativa - não pode reprovar inscrições")
	}

	event := &InscricaoReprovadaPorAcademiaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: a.ID,
		},
		EstudanteID: estudanteID,
		InscricaoID: inscricaoID,
		AcademiaID:  a.ID,
		Motivo:      motivo,
		RejectedAt:  time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento InscricaoReprovada gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) Ativar() error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando Ativar: id=%s, status_atual=%s", a.ID, a.Status)
	
	if a.Status == "ativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia já está ativa")
		return fmt.Errorf("academia já está ativa")
	}

	event := &AcademiaAtivadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaAtivada",
			AggregateID: a.ID,
		},
		ActivatedAt: time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento AcademiaAtivada gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) Desativar(motivo string) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando Desativar: id=%s, motivo=%s", a.ID, motivo)
	
	if a.Status == "inativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia já está inativa")
		return fmt.Errorf("academia já está inativa")
	}

	event := &AcademiaDesativadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaDesativada",
			AggregateID: a.ID,
		},
		Motivo:        motivo,
		DeactivatedAt: time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento AcademiaDesativada gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarCursos(cursos []string) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando AtualizarCursos: id=%s, cursos=%v", a.ID, cursos)
	
	if a.Status != "ativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia inativa")
		return fmt.Errorf("academia está inativa")
	}

	event := &CursosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursosAtualizados",
			AggregateID: a.ID,
		},
		NovoCursos: cursos,
		UpdatedAt:  time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento CursosAtualizados gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarDados(
	nome *string,
	provincia *string,
	endereco *string,
	numeroTelefone *string,
	email *string,
	website *string,
	nivelEscolar *string,
	cursos []string,
) error {
	log.Printf("[ACADEMIA_AGGREGATE] Executando comando AtualizarDados: id=%s", a.ID)
	
	if a.Status != "ativo" && a.Status != "inativo" {
		log.Printf("[ACADEMIA_AGGREGATE] Academia em estado inválido: %s", a.Status)
		return fmt.Errorf("academia em estado inválido")
	}

	if nome == nil && provincia == nil && endereco == nil && numeroTelefone == nil && 
	   email == nil && website == nil && nivelEscolar == nil && cursos == nil {
		log.Printf("[ACADEMIA_AGGREGATE] Nenhum campo fornecido para atualização")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if nome != nil && *nome == "" {
		log.Printf("[ACADEMIA_AGGREGATE] Nome vazio fornecido")
		return fmt.Errorf("nome não pode ser vazio")
	}

	if endereco != nil && *endereco == "" {
		log.Printf("[ACADEMIA_AGGREGATE] Endereço vazio fornecido")
		return fmt.Errorf("endereço não pode ser vazio")
	}

	if nivelEscolar != nil && a.Type == "escola" {
		validNiveis := map[string]bool{
			"fundamental": true,
			"medio":       true,
			"misto":       true,
		}
		if !validNiveis[*nivelEscolar] {
			log.Printf("[ACADEMIA_AGGREGATE] Nível escolar inválido: %s", *nivelEscolar)
			return fmt.Errorf("nivel_escolar deve ser 'fundamental', 'medio' ou 'misto'")
		}
	}

	event := &AcademiaDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaDadosAtualizados",
			AggregateID: a.ID,
		},
		Nome:           nome,
		Provincia:      provincia,
		Endereco:       endereco,
		NumeroTelefone: numeroTelefone,
		Email:          email,
		Website:        website,
		NivelEscolar:   nivelEscolar,
		Cursos:         cursos,
		EmailAlterado:  email != nil && (a.Email == nil || *a.Email != *email),
		UpdatedAt:      time.Now(),
	}

	log.Printf("[ACADEMIA_AGGREGATE] Evento AcademiaDadosAtualizados gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

// Event Handlers

func (a *Academia) applyAcademiaCriada(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando AcademiaCriada")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev AcademiaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	a.ID = event.GetAggregateID()
	a.Type = ev.Type
	a.Nome = ev.Nome
	a.CodigoAcademia = ev.CodigoAcademia
	a.SenhaHash = ev.SenhaHash
	a.Provincia = ev.Provincia
	a.Endereco = ev.Endereco
	a.NumeroTelefone = ev.NumeroTelefone
	a.Email = ev.Email
	a.Website = ev.Website
	a.NivelEscolar = ev.NivelEscolar
	a.Cursos = ev.Cursos
	a.Status = "inativo"
	a.CreatedAt = ev.CreatedAt

	log.Printf("[ACADEMIA_AGGREGATE] Estado atualizado: nome=%s, codigo=%s, status=%s", 
		a.Nome, a.CodigoAcademia, a.Status)
	return nil
}

func (a *Academia) applyInscricaoAprovada(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando InscricaoAprovada")
	
	a.TotalInscricoesPendentes--
	if a.TotalInscricoesPendentes < 0 {
		a.TotalInscricoesPendentes = 0
	}
	
	log.Printf("[ACADEMIA_AGGREGATE] Total inscrições pendentes: %d", a.TotalInscricoesPendentes)
	return nil
}

func (a *Academia) applyInscricaoReprovada(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando InscricaoReprovada")
	
	a.TotalInscricoesPendentes--
	if a.TotalInscricoesPendentes < 0 {
		a.TotalInscricoesPendentes = 0
	}
	
	log.Printf("[ACADEMIA_AGGREGATE] Total inscrições pendentes: %d", a.TotalInscricoesPendentes)
	return nil
}

func (a *Academia) applyAcademiaAtivada(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando AcademiaAtivada")
	a.Status = "ativo"
	log.Printf("[ACADEMIA_AGGREGATE] Status atualizado: %s", a.Status)
	return nil
}

func (a *Academia) applyAcademiaDesativada(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando AcademiaDesativada")
	a.Status = "inativo"
	log.Printf("[ACADEMIA_AGGREGATE] Status atualizado: %s", a.Status)
	return nil
}

func (a *Academia) applyCursosAtualizados(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando CursosAtualizados")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev CursosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	a.Cursos = ev.NovoCursos
	log.Printf("[ACADEMIA_AGGREGATE] Cursos atualizados: %v", a.Cursos)
	return nil
}

func (a *Academia) applyAcademiaDadosAtualizados(event DomainEvent) error {
	log.Printf("[ACADEMIA_AGGREGATE] Aplicando AcademiaDadosAtualizados")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev AcademiaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ACADEMIA_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	if ev.Nome != nil {
		a.Nome = *ev.Nome
		log.Printf("[ACADEMIA_AGGREGATE] Nome atualizado: %s", a.Nome)
	}
	if ev.Provincia != nil {
		a.Provincia = *ev.Provincia
		log.Printf("[ACADEMIA_AGGREGATE] Província atualizada: %s", a.Provincia)
	}
	if ev.Endereco != nil {
		a.Endereco = *ev.Endereco
		log.Printf("[ACADEMIA_AGGREGATE] Endereço atualizado: %s", a.Endereco)
	}
	if ev.NumeroTelefone != nil {
		a.NumeroTelefone = ev.NumeroTelefone
	}
	if ev.Email != nil {
		a.Email = ev.Email
		log.Printf("[ACADEMIA_AGGREGATE] Email atualizado")
	}
	if ev.Website != nil {
		a.Website = ev.Website
	}
	if ev.NivelEscolar != nil {
		a.NivelEscolar = ev.NivelEscolar
		log.Printf("[ACADEMIA_AGGREGATE] Nível escolar atualizado")
	}
	if ev.Cursos != nil {
		a.Cursos = ev.Cursos
		log.Printf("[ACADEMIA_AGGREGATE] Cursos atualizados: %v", a.Cursos)
	}

	return nil
}

// Eventos

type AcademiaCriadaEvent struct {
	BaseEvent
	Type           string
	Nome           string
	CodigoAcademia string
	SenhaHash      string
	Provincia      string
	Endereco       string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Cursos         []string
	CreatedAt      time.Time
}

func (e *AcademiaCriadaEvent) GetPayload() interface{} {
	return e
}

type InscricaoAprovadaPorAcademiaEvent struct {
	BaseEvent
	EstudanteID  uuid.UUID
	InscricaoID  uuid.UUID
	AcademiaID   uuid.UUID
	Tipo         string
	AnoInscricao string
	Curso        *string
	ApprovedAt   time.Time
}

func (e *InscricaoAprovadaPorAcademiaEvent) GetPayload() interface{} {
	return e
}

type InscricaoReprovadaPorAcademiaEvent struct {
	BaseEvent
	EstudanteID uuid.UUID
	InscricaoID uuid.UUID
	AcademiaID  uuid.UUID
	Motivo      string
	RejectedAt  time.Time
}

func (e *InscricaoReprovadaPorAcademiaEvent) GetPayload() interface{} {
	return e
}

type AcademiaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *AcademiaAtivadaEvent) GetPayload() interface{} {
	return e
}

type AcademiaDesativadaEvent struct {
	BaseEvent
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AcademiaDesativadaEvent) GetPayload() interface{} {
	return e
}

type CursosAtualizadosEvent struct {
	BaseEvent
	NovoCursos []string
	UpdatedAt  time.Time
}

func (e *CursosAtualizadosEvent) GetPayload() interface{} {
	return e
}

type AcademiaDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	Provincia      *string
	Endereco       *string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Cursos         []string
	EmailAlterado  bool
	UpdatedAt      time.Time
}

func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} {
	return e
}