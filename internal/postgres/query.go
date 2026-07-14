package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/CTran10/clearance/internal/transaction"
	"github.com/jackc/pgx/v5"
)

const transactionDetailColumns = `
	id, kind, account_id, coalesce(merchant_id, ''), coalesce(funding_source, ''),
	coalesce(external_reference, ''), amount_cents, currency, status,
	coalesce(risk_level, ''), coalesce(risk_reason, ''), correlation_id, created_at, updated_at`

func (s *Store) GetTransaction(ctx context.Context, id string) (transaction.Detail, bool, error) {
	var detail transaction.Detail
	err := s.pool.QueryRow(
		ctx,
		`select `+transactionDetailColumns+` from transactions where id = $1`,
		id,
	).Scan(detailDestinations(&detail)...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return transaction.Detail{}, false, nil
		}
		return transaction.Detail{}, false, fmt.Errorf("query transaction detail: %w", err)
	}
	return detail, true, nil
}

func (s *Store) ListTransactions(ctx context.Context, filter transaction.StoreListFilter) ([]transaction.Detail, error) {
	rows, err := s.pool.Query(
		ctx,
		`select `+transactionDetailColumns+`
		   from transactions
		  where account_id = $1
		    and ($2 = '' or status = $2)
		    and ($3 = '' or kind = $3)
		    and ($4::timestamptz is null or (created_at, id) < ($4, $5))
		  order by created_at desc, id desc
		  limit $6`,
		filter.AccountID,
		filter.Status,
		filter.Kind,
		nullTime(filter.BeforeCreatedAt),
		filter.BeforeID,
		filter.Limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("query transaction list: %w", err)
	}
	defer rows.Close()

	items := make([]transaction.Detail, 0, filter.Limit+1)
	for rows.Next() {
		var detail transaction.Detail
		if err := rows.Scan(detailDestinations(&detail)...); err != nil {
			return nil, fmt.Errorf("scan transaction list: %w", err)
		}
		items = append(items, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction list: %w", err)
	}
	return items, nil
}

func detailDestinations(detail *transaction.Detail) []any {
	return []any{
		&detail.ID,
		&detail.Kind,
		&detail.AccountID,
		&detail.MerchantID,
		&detail.FundingSource,
		&detail.ExternalReference,
		&detail.AmountCents,
		&detail.Currency,
		&detail.Status,
		&detail.RiskLevel,
		&detail.RiskReason,
		&detail.CorrelationID,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	}
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
