package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GraphStore struct {
	db *pgxpool.Pool
}

func NewGraphStore(db *pgxpool.Pool) *GraphStore {
	return &GraphStore{db: db}
}

func (s *GraphStore) CreateEdge(ctx context.Context, edge *domain.GraphEdge) error {
	// Create primary edge
	err := s.db.QueryRow(ctx,
		`INSERT INTO memory_graph (source_id, target_id, relation_type, strength)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (source_id, target_id, relation_type) DO UPDATE
		 SET strength = GREATEST(memory_graph.strength, EXCLUDED.strength),
		     traversal_count = memory_graph.traversal_count
		 RETURNING id, created_at, last_traversed_at, traversal_count`,
		edge.SourceID, edge.TargetID, edge.RelationType, edge.Strength,
	).Scan(&edge.ID, &edge.CreatedAt, &edge.LastTraversedAt, &edge.TraversalCount)
	if err != nil {
		return err
	}

	// For symmetric relations, create reverse edge
	if domain.SymmetricRelations[edge.RelationType] && edge.SourceID != edge.TargetID {
		_, _ = s.db.Exec(ctx,
			`INSERT INTO memory_graph (source_id, target_id, relation_type, strength)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (source_id, target_id, relation_type) DO UPDATE
			 SET strength = GREATEST(memory_graph.strength, EXCLUDED.strength)`,
			edge.TargetID, edge.SourceID, edge.RelationType, edge.Strength,
		)
	}

	return nil
}

func (s *GraphStore) GetEdge(ctx context.Context, sourceID, targetID uuid.UUID, relationType domain.RelationType) (*domain.GraphEdge, error) {
	edge := &domain.GraphEdge{}
	err := s.db.QueryRow(ctx,
		`SELECT id, source_id, target_id, relation_type, strength, created_at, last_traversed_at, traversal_count
		 FROM memory_graph
		 WHERE source_id = $1 AND target_id = $2 AND relation_type = $3`,
		sourceID, targetID, relationType,
	).Scan(&edge.ID, &edge.SourceID, &edge.TargetID, &edge.RelationType, &edge.Strength,
		&edge.CreatedAt, &edge.LastTraversedAt, &edge.TraversalCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return edge, nil
}

func (s *GraphStore) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM memory_graph WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *GraphStore) GetNeighbors(ctx context.Context, memoryID uuid.UUID, direction string, relationTypes []domain.RelationType) ([]domain.GraphEdge, error) {
	var query string
	var args []any

	switch direction {
	case "outgoing":
		query = `SELECT id, source_id, target_id, relation_type, strength, created_at, last_traversed_at, traversal_count
				 FROM memory_graph WHERE source_id = $1`
		args = append(args, memoryID)
	case "incoming":
		query = `SELECT id, source_id, target_id, relation_type, strength, created_at, last_traversed_at, traversal_count
				 FROM memory_graph WHERE target_id = $1`
		args = append(args, memoryID)
	default: // "both"
		query = `SELECT id, source_id, target_id, relation_type, strength, created_at, last_traversed_at, traversal_count
				 FROM memory_graph WHERE source_id = $1 OR target_id = $1`
		args = append(args, memoryID)
	}

	if len(relationTypes) > 0 {
		types := make([]string, len(relationTypes))
		for i, rt := range relationTypes {
			types[i] = string(rt)
		}
		query += fmt.Sprintf(" AND relation_type = ANY($%d)", len(args)+1)
		args = append(args, types)
	}

	query += " ORDER BY strength DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []domain.GraphEdge
	for rows.Next() {
		var edge domain.GraphEdge
		if err := rows.Scan(&edge.ID, &edge.SourceID, &edge.TargetID, &edge.RelationType,
			&edge.Strength, &edge.CreatedAt, &edge.LastTraversedAt, &edge.TraversalCount); err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func (s *GraphStore) UpdateTraversalStats(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memory_graph
		 SET last_traversed_at = NOW(), traversal_count = traversal_count + 1
		 WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *GraphStore) GetEdgesBySource(ctx context.Context, sourceID uuid.UUID) ([]domain.GraphEdge, error) {
	return s.GetNeighbors(ctx, sourceID, "outgoing", nil)
}

func (s *GraphStore) GetEdgesByTarget(ctx context.Context, targetID uuid.UUID) ([]domain.GraphEdge, error) {
	return s.GetNeighbors(ctx, targetID, "incoming", nil)
}

func (s *GraphStore) DeleteEdgesByMemory(ctx context.Context, memoryID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM memory_graph WHERE source_id = $1 OR target_id = $1`,
		memoryID,
	)
	return err
}

func (s *GraphStore) RecordTraversal(ctx context.Context, edgeID uuid.UUID, strengthBoost float32) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE memory_graph
		 SET last_traversed_at = NOW(),
		     traversal_count = traversal_count + 1,
		     strength = LEAST(strength + $2, 1.0)
		 WHERE id = $1`,
		edgeID, strengthBoost,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *GraphStore) ApplyEdgeDecay(ctx context.Context, agentID uuid.UUID, decayRate float64) (*domain.EdgeDecayResult, error) {
	result := &domain.EdgeDecayResult{}

	// Apply exponential decay based on time since last traversal
	// strength_new = strength * exp(-λ * hours_since_traversal)
	tag, err := s.db.Exec(ctx,
		`UPDATE memory_graph mg
		 SET strength = GREATEST(
		     strength * exp(-$2 * EXTRACT(EPOCH FROM (NOW() - COALESCE(last_traversed_at, created_at))) / 3600),
		     0.05
		 )
		 FROM memories m
		 WHERE mg.source_id = m.id
		   AND m.agent_id = $1
		   AND COALESCE(mg.last_traversed_at, mg.created_at) < NOW() - INTERVAL '1 hour'`,
		agentID, decayRate,
	)
	if err != nil {
		return nil, err
	}
	result.Decayed = int(tag.RowsAffected())
	result.Processed = result.Decayed

	return result, nil
}

func (s *GraphStore) PruneGraph(ctx context.Context, agentID uuid.UUID, rules domain.PruningRules) (*domain.EdgeDecayResult, error) {
	result := &domain.EdgeDecayResult{}

	// 1. Delete weak edges
	tag, err := s.db.Exec(ctx,
		`DELETE FROM memory_graph mg
		 USING memories m
		 WHERE mg.source_id = m.id
		   AND m.agent_id = $1
		   AND mg.strength < $2`,
		agentID, rules.StrengthThreshold,
	)
	if err != nil {
		return nil, err
	}
	result.Pruned += int(tag.RowsAffected())

	// 2. Delete stale edges (not traversed in threshold period and low traversal count)
	if rules.StaleThreshold > 0 {
		tag, err = s.db.Exec(ctx,
			`DELETE FROM memory_graph mg
			 USING memories m
			 WHERE mg.source_id = m.id
			   AND m.agent_id = $1
			   AND COALESCE(mg.last_traversed_at, mg.created_at) < NOW() - $2::INTERVAL
			   AND mg.traversal_count < 3`,
			agentID, fmt.Sprintf("%d hours", int(rules.StaleThreshold.Hours())),
		)
		if err != nil {
			return nil, err
		}
		result.Pruned += int(tag.RowsAffected())
	}

	// 3. Keep only top N edges per memory (by strength)
	if rules.MaxEdgesPerMemory > 0 {
		tag, err = s.db.Exec(ctx,
			`DELETE FROM memory_graph mg
			 WHERE mg.id IN (
			     SELECT mg2.id FROM memory_graph mg2
			     JOIN memories m ON mg2.source_id = m.id
			     WHERE m.agent_id = $1
			       AND mg2.id NOT IN (
			           SELECT id FROM (
			               SELECT id, ROW_NUMBER() OVER (
			                   PARTITION BY source_id
			                   ORDER BY strength DESC
			               ) as rn
			               FROM memory_graph
			           ) ranked
			           WHERE rn <= $2
			       )
			 )`,
			agentID, rules.MaxEdgesPerMemory,
		)
		if err != nil {
			return nil, err
		}
		result.Pruned += int(tag.RowsAffected())
	}

	result.Processed = result.Pruned
	return result, nil
}
