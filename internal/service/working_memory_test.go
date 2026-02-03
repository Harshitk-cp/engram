package service

import (
	"context"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockWorkingMemoryStore mocks the WorkingMemoryStore interface.
type MockWorkingMemoryStore struct {
	mock.Mock
}

func (m *MockWorkingMemoryStore) CreateSession(ctx context.Context, s *domain.WorkingMemorySession) error {
	args := m.Called(ctx, s)
	if args.Get(0) != nil {
		s.ID = uuid.New()
	}
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) GetSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) (*domain.WorkingMemorySession, error) {
	args := m.Called(ctx, agentID, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.WorkingMemorySession), args.Error(1)
}

func (m *MockWorkingMemoryStore) GetSessionByID(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (*domain.WorkingMemorySession, error) {
	args := m.Called(ctx, id, tenantID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.WorkingMemorySession), args.Error(1)
}

func (m *MockWorkingMemoryStore) UpdateSession(ctx context.Context, s *domain.WorkingMemorySession) error {
	args := m.Called(ctx, s)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) DeleteSession(ctx context.Context, agentID uuid.UUID, tenantID uuid.UUID) error {
	args := m.Called(ctx, agentID, tenantID)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) UpdateLastActivity(ctx context.Context, sessionID uuid.UUID) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) CreateActivation(ctx context.Context, a *domain.WorkingMemoryActivation) error {
	args := m.Called(ctx, a)
	if args.Get(0) == nil {
		a.ID = uuid.New()
	}
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) GetActivations(ctx context.Context, sessionID uuid.UUID) ([]domain.WorkingMemoryActivation, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.WorkingMemoryActivation), args.Error(1)
}

func (m *MockWorkingMemoryStore) ClearActivations(ctx context.Context, sessionID uuid.UUID) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) DeleteActivation(ctx context.Context, sessionID uuid.UUID, memoryType domain.ActivatedMemoryType, memoryID uuid.UUID) error {
	args := m.Called(ctx, sessionID, memoryType, memoryID)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) CreateSchemaActivation(ctx context.Context, a *domain.SchemaActivation) error {
	args := m.Called(ctx, a)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) GetSchemaActivations(ctx context.Context, sessionID uuid.UUID) ([]domain.SchemaActivation, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SchemaActivation), args.Error(1)
}

func (m *MockWorkingMemoryStore) ClearSchemaActivations(ctx context.Context, sessionID uuid.UUID) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockWorkingMemoryStore) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// MockMemoryAssociationStore mocks the MemoryAssociationStore interface.
type MockMemoryAssociationStore struct {
	mock.Mock
}

func (m *MockMemoryAssociationStore) Create(ctx context.Context, a *domain.MemoryAssociation) error {
	args := m.Called(ctx, a)
	return args.Error(0)
}

func (m *MockMemoryAssociationStore) GetBySource(ctx context.Context, sourceType domain.ActivatedMemoryType, sourceID uuid.UUID) ([]domain.MemoryAssociation, error) {
	args := m.Called(ctx, sourceType, sourceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.MemoryAssociation), args.Error(1)
}

func (m *MockMemoryAssociationStore) GetByTarget(ctx context.Context, targetType domain.ActivatedMemoryType, targetID uuid.UUID) ([]domain.MemoryAssociation, error) {
	args := m.Called(ctx, targetType, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.MemoryAssociation), args.Error(1)
}

func (m *MockMemoryAssociationStore) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockMemoryAssociationStore) UpdateStrength(ctx context.Context, id uuid.UUID, strength float32) error {
	args := m.Called(ctx, id, strength)
	return args.Error(0)
}

func TestWorkingMemoryService_Activate_CreatesNewSession(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)
	assocStore := new(MockMemoryAssociationStore)

	agentID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()

	// Session doesn't exist
	wmStore.On("GetSession", ctx, agentID, tenantID).Return(nil, store.ErrNotFound)

	// Create new session
	wmStore.On("CreateSession", ctx, mock.AnythingOfType("*domain.WorkingMemorySession")).
		Run(func(args mock.Arguments) {
			sess := args.Get(1).(*domain.WorkingMemorySession)
			sess.ID = sessionID
		}).
		Return(nil)

	// Clear activations
	wmStore.On("ClearActivations", ctx, sessionID).Return(nil)
	wmStore.On("ClearSchemaActivations", ctx, sessionID).Return(nil)

	// Update session
	wmStore.On("UpdateSession", ctx, mock.AnythingOfType("*domain.WorkingMemorySession")).Return(nil)

	svc := NewWorkingMemoryService(wmStore, assocStore, nil, nil, nil, nil, nil, logger)

	input := domain.ActivationInput{
		AgentID:  agentID,
		TenantID: tenantID,
		Goal:     "help user with debugging",
		Cues:     []string{"error", "debugging"},
	}

	result, err := svc.Activate(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, sessionID, result.Session.ID)
	assert.Equal(t, "help user with debugging", result.Session.CurrentGoal)
	assert.Equal(t, DefaultMaxSlots, result.MaxSlots)

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_Activate_UsesExistingSession(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)
	assocStore := new(MockMemoryAssociationStore)

	agentID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()

	existingSession := &domain.WorkingMemorySession{
		ID:          sessionID,
		AgentID:     agentID,
		TenantID:    tenantID,
		CurrentGoal: "previous goal",
		MaxSlots:    7,
	}

	// Session exists
	wmStore.On("GetSession", ctx, agentID, tenantID).Return(existingSession, nil)

	// Clear and save activations
	wmStore.On("ClearActivations", ctx, sessionID).Return(nil)
	wmStore.On("ClearSchemaActivations", ctx, sessionID).Return(nil)
	wmStore.On("UpdateSession", ctx, mock.AnythingOfType("*domain.WorkingMemorySession")).Return(nil)

	svc := NewWorkingMemoryService(wmStore, assocStore, nil, nil, nil, nil, nil, logger)

	input := domain.ActivationInput{
		AgentID:  agentID,
		TenantID: tenantID,
		Goal:     "new goal",
		Cues:     []string{"test"},
	}

	result, err := svc.Activate(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, sessionID, result.Session.ID)
	assert.Equal(t, "new goal", result.Session.CurrentGoal) // Goal should be updated

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_GetSession_Found(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)

	agentID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()

	existingSession := &domain.WorkingMemorySession{
		ID:          sessionID,
		AgentID:     agentID,
		TenantID:    tenantID,
		CurrentGoal: "test goal",
		MaxSlots:    7,
	}

	wmStore.On("GetSession", ctx, agentID, tenantID).Return(existingSession, nil)
	wmStore.On("GetActivations", ctx, sessionID).Return([]domain.WorkingMemoryActivation{}, nil)
	wmStore.On("GetSchemaActivations", ctx, sessionID).Return([]domain.SchemaActivation{}, nil)

	svc := NewWorkingMemoryService(wmStore, nil, nil, nil, nil, nil, nil, logger)

	session, err := svc.GetSession(ctx, agentID, tenantID)

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, "test goal", session.CurrentGoal)

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_GetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)

	agentID := uuid.New()
	tenantID := uuid.New()

	wmStore.On("GetSession", ctx, agentID, tenantID).Return(nil, store.ErrNotFound)

	svc := NewWorkingMemoryService(wmStore, nil, nil, nil, nil, nil, nil, logger)

	session, err := svc.GetSession(ctx, agentID, tenantID)

	assert.Error(t, err)
	assert.Equal(t, ErrSessionNotFound, err)
	assert.Nil(t, session)

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_ClearSession(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)

	agentID := uuid.New()
	tenantID := uuid.New()

	wmStore.On("DeleteSession", ctx, agentID, tenantID).Return(nil)

	svc := NewWorkingMemoryService(wmStore, nil, nil, nil, nil, nil, nil, logger)

	err := svc.ClearSession(ctx, agentID, tenantID)

	assert.NoError(t, err)

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_UpdateGoal(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	wmStore := new(MockWorkingMemoryStore)

	agentID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()

	existingSession := &domain.WorkingMemorySession{
		ID:          sessionID,
		AgentID:     agentID,
		TenantID:    tenantID,
		CurrentGoal: "old goal",
		MaxSlots:    7,
	}

	wmStore.On("GetSession", ctx, agentID, tenantID).Return(existingSession, nil)
	wmStore.On("UpdateSession", ctx, mock.MatchedBy(func(s *domain.WorkingMemorySession) bool {
		return s.CurrentGoal == "new goal"
	})).Return(nil)

	svc := NewWorkingMemoryService(wmStore, nil, nil, nil, nil, nil, nil, logger)

	err := svc.UpdateGoal(ctx, agentID, tenantID, "new goal")

	assert.NoError(t, err)

	wmStore.AssertExpectations(t)
}

func TestWorkingMemoryService_Compete(t *testing.T) {
	logger := zap.NewNop()
	svc := NewWorkingMemoryService(nil, nil, nil, nil, nil, nil, nil, logger)

	items := []activatedItem{
		{Type: domain.ActivatedMemoryTypeSemantic, ID: uuid.New(), ActivationLevel: 0.5, Confidence: 0.8},
		{Type: domain.ActivatedMemoryTypeSemantic, ID: uuid.New(), ActivationLevel: 0.9, Confidence: 0.6},
		{Type: domain.ActivatedMemoryTypeSemantic, ID: uuid.New(), ActivationLevel: 0.7, Confidence: 0.9},
		{Type: domain.ActivatedMemoryTypeSemantic, ID: uuid.New(), ActivationLevel: 0.3, Confidence: 0.5},
	}

	// Test with maxSlots = 2
	winners := svc.compete(items, 2)

	assert.Len(t, winners, 2)

	// Should be sorted by (activation_level * confidence) descending
	// Item 0: 0.5 * 0.8 = 0.40
	// Item 1: 0.9 * 0.6 = 0.54
	// Item 2: 0.7 * 0.9 = 0.63 <- highest
	// Item 3: 0.3 * 0.5 = 0.15

	// Winners should be items with highest effective scores
	assert.Equal(t, float32(0.63), winners[0].ActivationLevel*winners[0].Confidence)
	assert.Equal(t, float32(0.54), winners[1].ActivationLevel*winners[1].Confidence)
}

func TestWorkingMemoryService_MergeActivations(t *testing.T) {
	logger := zap.NewNop()
	svc := NewWorkingMemoryService(nil, nil, nil, nil, nil, nil, nil, logger)

	id1 := uuid.New()
	id2 := uuid.New()

	a := []activatedItem{
		{Type: domain.ActivatedMemoryTypeSemantic, ID: id1, ActivationLevel: 0.5, Confidence: 0.8},
	}

	b := []activatedItem{
		{Type: domain.ActivatedMemoryTypeSemantic, ID: id1, ActivationLevel: 0.7, Confidence: 0.8}, // Duplicate with higher activation
		{Type: domain.ActivatedMemoryTypeSemantic, ID: id2, ActivationLevel: 0.6, Confidence: 0.9}, // New item
	}

	merged := svc.mergeActivations(a, b, 1.0)

	assert.Len(t, merged, 2)

	// Find the merged item with id1
	var found bool
	for _, item := range merged {
		if item.ID == id1 {
			assert.Equal(t, float32(0.7), item.ActivationLevel) // Should take higher activation
			found = true
		}
	}
	assert.True(t, found)
}

func TestWorkingMemoryService_AssembleContext(t *testing.T) {
	logger := zap.NewNop()
	svc := NewWorkingMemoryService(nil, nil, nil, nil, nil, nil, nil, logger)

	items := []activatedItem{
		{Type: domain.ActivatedMemoryTypeSemantic, ID: uuid.New(), Content: "User prefers dark mode", Confidence: 0.9},
		{Type: domain.ActivatedMemoryTypeEpisodic, ID: uuid.New(), Content: "User mentioned being frustrated with light theme", Confidence: 0.7},
		{Type: domain.ActivatedMemoryTypeProcedural, ID: uuid.New(), Content: "When: User mentions UI preferences\nDo: Suggest dark mode settings", Confidence: 0.8},
	}

	schemas := []domain.SchemaMatch{
		{Schema: domain.Schema{Name: "Power User", Description: "Prefers efficiency and dark themes"}, MatchScore: 0.8},
	}

	context := svc.assembleContext(items, schemas)

	assert.Contains(t, context, "Known Facts/Preferences")
	assert.Contains(t, context, "User prefers dark mode")
	assert.Contains(t, context, "Relevant Past Experiences")
	assert.Contains(t, context, "Applicable Patterns")
	assert.Contains(t, context, "Active Mental Models")
	assert.Contains(t, context, "Power User")
}

func TestWorkingMemoryService_CreateAssociation(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	assocStore := new(MockMemoryAssociationStore)

	assoc := &domain.MemoryAssociation{
		SourceMemoryType:    domain.ActivatedMemoryTypeEpisodic,
		SourceMemoryID:      uuid.New(),
		TargetMemoryType:    domain.ActivatedMemoryTypeSemantic,
		TargetMemoryID:      uuid.New(),
		AssociationType:     domain.AssociationTypeDerived,
		AssociationStrength: 0.8,
	}

	assocStore.On("Create", ctx, assoc).Return(nil)

	svc := NewWorkingMemoryService(nil, assocStore, nil, nil, nil, nil, nil, logger)

	err := svc.CreateAssociation(ctx, assoc)

	assert.NoError(t, err)
	assocStore.AssertExpectations(t)
}
