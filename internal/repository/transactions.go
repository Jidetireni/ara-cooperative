package repository

import (
	"context"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type TransactionRepository struct {
	db               *sqlx.DB
	psql             sq.StatementBuilderType
	memberRepository *MemberRepository
}

func NewTransactionRepository(db *sqlx.DB) *TransactionRepository {
	return &TransactionRepository{
		db:               db,
		psql:             sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		memberRepository: NewMemberRepository(db),
	}
}

type TransactionRepositoryFilter struct {
	ID         *uuid.UUID
	MemberID   *uuid.UUID
	StatusID   *uuid.UUID
	Confirmed  *bool
	Rejected   *bool
	Type       *TransactionType
	LedgerType *LedgerType
}

type PopulatedTransaction struct {
	Transaction
	Status TransactionStatus
	Member Member
}

type populateTransactionFlat struct {
	// transaction fields
	TrID          uuid.UUID       `json:"tr_id"`
	TrMemberID    uuid.UUID       `json:"tr_member_id"`
	TrDescription string          `json:"tr_description"`
	Tr_Reference  string          `json:"tr_reference"`
	TrAmount      int64           `json:"tr_amount"`
	TrType        TransactionType `json:"tr_type"`
	TrLedgerType  LedgerType      `json:"tr_ledger_type"`
	StatusID      uuid.UUID       `json:"tr_status_id"`
	TrCreatedAt   *time.Time      `json:"tr_created_at"`
	TrConfirmedAt *time.Time      `json:"tr_confirmed_at"`
	TrRejectedAt  *time.Time      `json:"tr_rejected_at"`

	//status fields
	TsID            uuid.UUID  `json:"ts_id"`
	TsTransactionID uuid.UUID  `json:"ts_transaction_id"`
	TsConfirmedAt   *time.Time `json:"ts_confirmed_at"`
	TsRejectedAt    *time.Time `json:"ts_rejected_at"`
	TsCreatedAt     *time.Time `json:"ts_created_at"`

	// member fields
	MbID             uuid.UUID  `json:"mb_id"`
	MbUserID         uuid.UUID  `json:"mb_user_id"`
	MbFirstName      string     `json:"mb_first_name"`
	MbLastName       string     `json:"mb_last_name"`
	MbSlug           string     `json:"mb_slug"`
	MbPhone          string     `json:"mb_phone"`
	MbAddress        *string    `json:"mb_address"`
	MbNextOfKinName  *string    `json:"mb_next_of_kin_name"`
	MbNextOfKinPhone *string    `json:"mb_next_of_kin_phone"`
	MbActivatedAt    *time.Time `json:"mb_activated_at"`
	MbCreatedAt      *time.Time `json:"mb_created_at"`
	MbUpdatedAt      *time.Time `json:"mb_updated_at"`
	MbDeletedAt      *time.Time `json:"mb_deleted_at"`
}

func (s *TransactionRepository) applyFilter(builder sq.SelectBuilder, filter TransactionRepositoryFilter) sq.SelectBuilder {
	// Now apply filters from the input `filter`
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"tr.id": *filter.ID})
	}

	if filter.Confirmed != nil {
		if *filter.Confirmed {
			builder = builder.Where("ts.confirmed_at IS NOT NULL")
		} else {
			builder = builder.Where("ts.confirmed_at IS NULL")
		}
	}

	if filter.Rejected != nil {
		if *filter.Rejected {
			builder = builder.Where("ts.rejected_at IS NOT NULL")
		} else {
			builder = builder.Where("ts.rejected_at IS NULL")
		}
	}

	if filter.StatusID != nil {
		builder = builder.Where(sq.Eq{"ts.id": *filter.StatusID})
	}

	if filter.Type != nil {
		builder = builder.Where(sq.Eq{"tr.type": *filter.Type})
	}

	if filter.MemberID != nil {
		builder = builder.Where(sq.Eq{"tr.member_id": *filter.MemberID})
	}

	if filter.LedgerType != nil {
		builder = builder.Where(sq.Eq{"tr.ledger": *filter.LedgerType})
	}

	return builder
}

func (s *TransactionRepository) buildQuery(filter TransactionRepositoryFilter, opts QueryOptions) (string, []interface{}, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	var err error

	var builder sq.SelectBuilder
	switch queryType {
	case QueryTypeSelect:
		builder = s.psql.Select(
			// transaction fields
			"tr.id AS tr_id",
			"tr.member_id AS tr_member_id",
			"tr.description AS tr_description",
			"tr.reference AS tr_reference",
			"tr.amount AS tr_amount",
			"tr.type AS tr_type",
			"tr.ledger AS tr_ledger_type",
			"tr.created_at AS tr_created_at",

			// transaction status fields
			"ts.id AS ts_id",
			"ts.transaction_id AS ts_transaction_id",
			"ts.confirmed_at AS ts_confirmed_at",
			"ts.rejected_at AS ts_rejected_at",
			"ts.created_at AS ts_created_at",

			// member fields
			"mb.id AS mb_id",
			"mb.user_id AS mb_user_id",
			"mb.first_name AS mb_first_name",
			"mb.last_name AS mb_last_name",
			"mb.slug AS mb_slug",
			"mb.phone AS mb_phone",
			"mb.address AS mb_address",
			"mb.next_of_kin_name AS mb_next_of_kin_name",
			"mb.next_of_kin_phone AS mb_next_of_kin_phone",
			"mb.activated_at AS mb_activated_at",
			"mb.created_at AS mb_created_at",
			"mb.updated_at AS mb_updated_at",
			"mb.deleted_at AS mb_deleted_at",
		)
	case QueryTypeCount:
		builder = s.psql.Select("COUNT(*)")
	}

	builder = builder.From("transactions tr")
	// Join the two tables on the transaction ID
	builder = builder.
		LeftJoin("transaction_status ts ON tr.id = ts.transaction_id").
		LeftJoin("members mb ON tr.member_id = mb.id")

	builder = s.applyFilter(builder, filter)

	if queryType != QueryTypeCount {
		if opts.Sort == nil {
			opts.Sort = lo.ToPtr("tr.created_at:desc")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}

	return builder.ToSql()
}

func (t TransactionRepository) Create(ctx context.Context, transaction Transaction, tx *sqlx.Tx) (*Transaction, error) {
	builder := t.psql.Insert("transactions").
		Columns("member_id", "description", "reference", "amount", "type", "ledger").
		Values(transaction.MemberID, transaction.Description, transaction.Reference, transaction.Amount, transaction.Type, transaction.Ledger).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdTransaction Transaction
	if tx != nil {
		err = tx.GetContext(ctx, &createdTransaction, query, args...)
		return &createdTransaction, err
	}

	err = t.db.GetContext(ctx, &createdTransaction, query, args...)
	return &createdTransaction, err
}

func (t TransactionRepository) GetPopulated(ctx context.Context, filter TransactionRepositoryFilter, tx *sqlx.Tx) (*PopulatedTransaction, error) {
	query, args, err := t.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}

	var popTxn populateTransactionFlat
	if tx != nil {
		err = tx.GetContext(ctx, &popTxn, query, args...)
		return t.mapFlatToPopulated(&popTxn), err
	}

	err = t.db.GetContext(ctx, &popTxn, query, args...)
	return t.mapFlatToPopulated(&popTxn), err
}

func (s *TransactionRepository) ListPopulated(ctx context.Context, filter TransactionRepositoryFilter, opts QueryOptions) (*ListResult[PopulatedTransaction], error) {
	query, args, err := s.buildQuery(filter, opts)
	if err != nil {
		return nil, err
	}

	var flatList []populateTransactionFlat
	err = s.db.SelectContext(ctx, &flatList, query, args...)
	if err != nil {
		return nil, err
	}

	populatedList := lo.Map(flatList, func(flat populateTransactionFlat, _ int) *PopulatedTransaction {
		return s.mapFlatToPopulated(&flat)
	})

	listResult := ListResult[PopulatedTransaction]{
		Items: lo.Slice(populatedList, 0, min(len(populatedList), int(opts.Limit))),
	}

	if len(populatedList) > int(opts.Limit) {
		lastItem := lo.LastOr(populatedList, nil)
		if lastItem != nil {
			nextCursor := EncodeCursor(lastItem.CreatedAt.Time, lastItem.ID)
			listResult.NextCursor = &nextCursor
		}
	}

	return &listResult, nil
}

func (s *TransactionRepository) GetStatus(ctx context.Context, filter TransactionRepositoryFilter) (*TransactionStatus, error) {
	builder := s.psql.Select(
		"ts.id",
		"ts.transaction_id",
		"ts.confirmed_at",
		"ts.rejected_at",
	).From("transaction_status ts").
		Join("transactions tr ON ts.transaction_id = tr.id")

	// Apply filters from the input `filter`
	builder = s.applyFilter(builder, filter)
	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var TransactionStatus TransactionStatus
	if err := s.db.GetContext(ctx, &TransactionStatus, query, args...); err != nil {
		return nil, err
	}

	return &TransactionStatus, nil
}

func (s *TransactionRepository) CreateStatus(ctx context.Context, transactionStatus TransactionStatus, tx *sqlx.Tx) (*TransactionStatus, error) {
	builder := s.psql.Insert("transaction_status").
		Columns("transaction_id", "confirmed_at", "rejected_at").
		Values(transactionStatus.TransactionID, transactionStatus.ConfirmedAt, transactionStatus.RejectedAt).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var createdStatus TransactionStatus
	if tx != nil {
		err = tx.GetContext(ctx, &createdStatus, query, args...)
		return &createdStatus, err
	}

	err = s.db.GetContext(ctx, &createdStatus, query, args...)
	return &createdStatus, err
}

func (s *TransactionRepository) GetBalance(ctx context.Context, filter TransactionRepositoryFilter) (int64, error) {
	builder := s.psql.Select("COALESCE(SUM(tr.amount), 0) AS balance").
		From("transactions tr").
		Join("transaction_status ts ON tr.id = ts.transaction_id").
		Where(sq.Eq{"tr.ledger": filter.LedgerType})

	builder = s.applyFilter(builder, filter)
	query, args, err := builder.ToSql()
	if err != nil {
		return 0, err
	}

	var balance int64
	if err := s.db.GetContext(ctx, &balance, query, args...); err != nil {
		return 0, err
	}

	return balance, nil
}

func (s *TransactionRepository) UpdateStatus(ctx context.Context, transactionStatus TransactionStatus, tx *sqlx.Tx) (*TransactionStatus, error) {
	builder := s.psql.Update("transaction_status").
		Set("confirmed_at", transactionStatus.ConfirmedAt).
		Set("rejected_at", transactionStatus.RejectedAt).
		Where(sq.Eq{"id": transactionStatus.ID}).
		Where(sq.Expr("confirmed_at IS NULL AND rejected_at IS NULL")).
		Suffix("RETURNING *")

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, err
	}

	var updatedTransactionStatus TransactionStatus
	if tx != nil {
		err = tx.GetContext(ctx, &updatedTransactionStatus, query, args...)
		return &updatedTransactionStatus, err
	}

	err = s.db.GetContext(ctx, &updatedTransactionStatus, query, args...)
	return &updatedTransactionStatus, err
}

func (t *TransactionRepository) mapTypeToDTOModel(txnType *TransactionType) *dto.TransactionType {
	if txnType != nil {
		var dtoTxnType dto.TransactionType
		switch *txnType {
		case TransactionTypeDEPOSIT:
			dtoTxnType = dto.TransactionTypeDeposit
		case TransactionTypeWITHDRAWAL:
			dtoTxnType = dto.TransactionTypeWithdrawal
		default:
			dtoTxnType = dto.TransactionTypeDeposit
		}

		return &dtoTxnType
	}
	return nil
}

func (t *TransactionRepository) mapLedgerToDTOModel(ledgerType *LedgerType) *dto.LedgerType {
	if ledgerType != nil {
		var dtoLedgerType dto.LedgerType
		switch *ledgerType {
		case LedgerTypeSAVINGS:
			dtoLedgerType = dto.LedgerTypeSAVINGS
		case LedgerTypeSPECIALDEPOSIT:
			dtoLedgerType = dto.LedgerTypeSPECIALDEPOSIT
		case LedgerTypeSHARES:
			dtoLedgerType = dto.LedgerTypeSHARES
		case LedgerTypeFINES:
			dtoLedgerType = dto.LedgerTypeFINES
		case LedgerTypeREGISTRATIONFEE:
			dtoLedgerType = dto.LedgerTypeREGISTRATIONFEE
		default:
			dtoLedgerType = dto.LedgerTypeSAVINGS
		}

		return &dtoLedgerType
	}
	return nil
}

func (t *TransactionRepository) mapStatusToDTOModel(status *TransactionStatus) *dto.TransactionStatus {
	if status != nil {
		DTOStatus := dto.TransactionStatusTypePending
		if status.ConfirmedAt.Valid {
			DTOStatus = dto.TransactionStatusTypeConfirmed
		} else if status.RejectedAt.Valid {
			DTOStatus = dto.TransactionStatusTypeRejected
		}

		return &dto.TransactionStatus{
			ID:          status.ID,
			Status:      DTOStatus,
			ConfirmedAt: &status.ConfirmedAt.Time,
			RejectedAt:  &status.RejectedAt.Time,
		}
	}

	return nil
}

func (s *TransactionRepository) mapFlatToPopulated(flat *populateTransactionFlat) *PopulatedTransaction {
	transaction := Transaction{
		ID:          flat.TrID,
		MemberID:    flat.TrMemberID,
		Description: flat.TrDescription,
		Reference:   flat.Tr_Reference,
		Amount:      flat.TrAmount,
		Type:        flat.TrType,
		Ledger:      flat.TrLedgerType,
		CreatedAt:   ToNullTime(flat.TrCreatedAt),
	}

	status := TransactionStatus{
		ID:            flat.StatusID,
		TransactionID: flat.TsTransactionID,
		ConfirmedAt:   ToNullTime(flat.TsConfirmedAt),
		RejectedAt:    ToNullTime(flat.TsRejectedAt),
		CreatedAt:     ToNullTime(flat.TsCreatedAt),
	}

	member := Member{
		ID:             flat.MbID,
		UserID:         flat.MbUserID,
		FirstName:      flat.MbFirstName,
		LastName:       flat.MbLastName,
		Slug:           flat.MbSlug,
		Phone:          flat.MbPhone,
		Address:        ToNullString(flat.MbAddress),
		NextOfKinName:  ToNullString(flat.MbNextOfKinName),
		NextOfKinPhone: ToNullString(flat.MbNextOfKinPhone),
		ActivatedAt:    ToNullTime(flat.MbActivatedAt),
		CreatedAt:      lo.FromPtrOr(flat.MbCreatedAt, time.Time{}),
		UpdatedAt:      ToNullTime(flat.MbUpdatedAt),
		DeletedAt:      ToNullTime(flat.MbDeletedAt),
	}

	return &PopulatedTransaction{
		Transaction: transaction,
		Status:      status,
		Member:      member,
	}
}

func (t *TransactionRepository) MapRepositoryToDTOModel(txn *PopulatedTransaction) *dto.Transactions {
	if txn != nil {
		return &dto.Transactions{
			ID:          txn.Transaction.ID,
			Description: txn.Transaction.Description,
			Reference:   txn.Transaction.Reference,
			Amount:      txn.Transaction.Amount,
			Type:        *t.mapTypeToDTOModel(&txn.Transaction.Type),
			LedgerType:  *t.mapLedgerToDTOModel(&txn.Transaction.Ledger),
			Status:      *t.mapStatusToDTOModel(&txn.Status),
			Member:      *t.memberRepository.MapRepositoryToDTOModel(&txn.Member),
		}
	}

	return nil
}
