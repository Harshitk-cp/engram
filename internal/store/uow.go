package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UnitOfWork runs operations on the audited stores (memory, mutation log,
// contradiction) atomically within a single transaction, so a state change and
// its audit-log row commit together or not at all.
type UnitOfWork struct {
	pool          *pgxpool.Pool
	memory        *MemoryStore
	mutationLog   *MutationLogStore
	contradiction *ContradictionStore
}

func NewUnitOfWork(pool *pgxpool.Pool, memory *MemoryStore, mutationLog *MutationLogStore, contradiction *ContradictionStore) *UnitOfWork {
	return &UnitOfWork{pool: pool, memory: memory, mutationLog: mutationLog, contradiction: contradiction}
}

// TxStores exposes the audited stores bound to one transaction.
type TxStores struct {
	Memory        *MemoryStore
	MutationLog   *MutationLogStore
	Contradiction *ContradictionStore
}

// Do runs fn with transaction-bound stores; all writes commit or roll back together.
func (u *UnitOfWork) Do(ctx context.Context, fn func(*TxStores) error) error {
	return WithTx(ctx, u.pool, func(tx pgx.Tx) error {
		return fn(&TxStores{
			Memory:        u.memory.withTx(tx),
			MutationLog:   u.mutationLog.withTx(tx),
			Contradiction: u.contradiction.withTx(tx),
		})
	})
}
