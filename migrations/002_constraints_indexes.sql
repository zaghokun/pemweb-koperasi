SET SESSION time_zone = '+00:00';
SET SESSION sql_mode = 'STRICT_ALL_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';
SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

ALTER TABLE loans
    ADD COLUMN open_loan_guard TINYINT
        GENERATED ALWAYS AS (
            CASE WHEN status IN ('PENDING', 'APPROVED', 'ACTIVE') THEN 1 ELSE NULL END
        ) VIRTUAL,
    ADD UNIQUE KEY uq_member_open_loan (member_id, open_loan_guard);

ALTER TABLE savings
    ADD COLUMN principal_deposit_guard TINYINT
        GENERATED ALWAYS AS (
            CASE WHEN saving_type = 'POKOK' AND transaction_type = 'SETOR' THEN 1 ELSE NULL END
        ) VIRTUAL,
    ADD COLUMN exit_refund_guard TINYINT
        GENERATED ALWAYS AS (
            CASE WHEN transaction_type = 'PENGEMBALIAN_KELUAR' THEN 1 ELSE NULL END
        ) VIRTUAL,
    ADD UNIQUE KEY uq_member_principal_deposit (member_id, principal_deposit_guard),
    ADD UNIQUE KEY uq_member_mandatory_period (member_id, period_month),
    ADD UNIQUE KEY uq_member_exit_refund_type (member_id, saving_type, exit_refund_guard);

-- FK-prefix query indexes and installment uniqueness are declared in 001,
-- before their FKs, so InnoDB does not create redundant supporting indexes.
-- The automatic loans.member_id FK index can be replaced by uq_member_open_loan.
CREATE INDEX idx_installments_due_date ON installments (due_date, loan_id);
CREATE INDEX idx_payments_monthly ON payments (payment_date, id);
