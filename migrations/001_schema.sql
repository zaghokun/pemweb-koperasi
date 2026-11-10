-- Run once on an empty MySQL 8.4 database; DDL implicitly commits.
SET SESSION time_zone = '+00:00';
SET SESSION sql_mode = 'STRICT_ALL_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';
SET NAMES utf8mb4 COLLATE utf8mb4_0900_ai_ci;

CREATE TABLE users (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    username VARCHAR(50) COLLATE utf8mb4_0900_bin NOT NULL,
    password_hash TEXT NOT NULL,
    is_active BOOLEAN NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_users_username (username),
    CONSTRAINT chk_users_is_active CHECK (is_active IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE sessions (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    token_hash BINARY(32) NOT NULL,
    user_id BIGINT NULL,
    csrf_token CHAR(64) COLLATE utf8mb4_0900_bin NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    expires_at DATETIME(6) NOT NULL,
    UNIQUE KEY uq_sessions_token_hash (token_hash),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_sessions_expiry CHECK (expires_at > created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE members (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    member_number VARCHAR(30) COLLATE utf8mb4_0900_bin NOT NULL,
    name VARCHAR(100) NOT NULL,
    address TEXT NOT NULL,
    phone VARCHAR(20) NOT NULL,
    join_date DATE NOT NULL,
    status VARCHAR(10) COLLATE utf8mb4_0900_bin NOT NULL,
    exit_date DATE NULL,
    exit_user_id BIGINT NULL,
    created_by BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_members_number (member_number),
    CONSTRAINT fk_members_exit_user FOREIGN KEY (exit_user_id) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_members_creator FOREIGN KEY (created_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_members_status CHECK (status IN ('ACTIVE', 'EXITED')),
    -- IS NOT NULL prevents UNKNOWN from accepting an incomplete exit state.
    CONSTRAINT chk_members_exit CHECK (
        (status = 'ACTIVE' AND exit_date IS NULL AND exit_user_id IS NULL)
        OR (status = 'EXITED' AND exit_date IS NOT NULL AND exit_user_id IS NOT NULL
            AND exit_date >= join_date)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE savings (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    member_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    saving_type VARCHAR(10) COLLATE utf8mb4_0900_bin NOT NULL,
    transaction_type VARCHAR(25) COLLATE utf8mb4_0900_bin NOT NULL,
    amount BIGINT NOT NULL,
    period_month DATE NULL,
    transaction_date DATE NOT NULL,
    request_key CHAR(32) COLLATE utf8mb4_0900_bin NOT NULL,
    description TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_savings_request_key (request_key),
    KEY idx_savings_member_date (member_id, transaction_date, id),
    CONSTRAINT fk_savings_member FOREIGN KEY (member_id) REFERENCES members (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_savings_user FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_savings_type CHECK (saving_type IN ('POKOK', 'WAJIB', 'SUKARELA')),
    CONSTRAINT chk_savings_transaction CHECK (
        transaction_type IN ('SETOR', 'TARIK', 'PENGEMBALIAN_KELUAR')
    ),
    CONSTRAINT chk_savings_amount CHECK (amount > 0),
    CONSTRAINT chk_savings_principal CHECK (
        NOT (saving_type = 'POKOK' AND transaction_type = 'SETOR') OR amount = 500000
    ),
    CONSTRAINT chk_savings_mandatory CHECK (
        NOT (saving_type = 'WAJIB' AND transaction_type = 'SETOR') OR amount = 100000
    ),
    CONSTRAINT chk_savings_withdrawal CHECK (
        transaction_type <> 'TARIK' OR saving_type = 'SUKARELA'
    ),
    CONSTRAINT chk_savings_period CHECK (
        (saving_type = 'WAJIB' AND transaction_type = 'SETOR'
            AND period_month IS NOT NULL AND DAYOFMONTH(period_month) = 1)
        OR ((saving_type <> 'WAJIB' OR transaction_type <> 'SETOR') AND period_month IS NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE loans (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    member_id BIGINT NOT NULL,
    principal BIGINT NOT NULL,
    tenor_months SMALLINT NOT NULL,
    application_date DATE NOT NULL,
    status VARCHAR(10) COLLATE utf8mb4_0900_bin NOT NULL,
    created_by BIGINT NOT NULL,
    approved_by BIGINT NULL,
    disbursed_by BIGINT NULL,
    rejected_by BIGINT NULL,
    disbursement_date DATE NULL,
    settled_on DATE NULL,
    rejection_reason TEXT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT fk_loans_member FOREIGN KEY (member_id) REFERENCES members (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_loans_creator FOREIGN KEY (created_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_loans_approver FOREIGN KEY (approved_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_loans_disburser FOREIGN KEY (disbursed_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_loans_rejector FOREIGN KEY (rejected_by) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_loans_principal CHECK (principal > 0),
    CONSTRAINT chk_loans_tenor CHECK (tenor_months BETWEEN 6 AND 24),
    CONSTRAINT chk_loans_status CHECK (status IN ('PENDING', 'APPROVED', 'ACTIVE', 'SETTLED', 'REJECTED')),
    CONSTRAINT chk_loans_lifecycle CHECK (
        (status = 'PENDING' AND approved_by IS NULL AND disbursed_by IS NULL
            AND rejected_by IS NULL AND disbursement_date IS NULL
            AND settled_on IS NULL AND rejection_reason IS NULL)
        OR (status = 'APPROVED' AND approved_by IS NOT NULL AND disbursed_by IS NULL
            AND rejected_by IS NULL AND disbursement_date IS NULL
            AND settled_on IS NULL AND rejection_reason IS NULL)
        OR (status = 'ACTIVE' AND approved_by IS NOT NULL AND disbursed_by IS NOT NULL
            AND rejected_by IS NULL AND disbursement_date IS NOT NULL
            AND settled_on IS NULL AND rejection_reason IS NULL)
        OR (status = 'SETTLED' AND approved_by IS NOT NULL AND disbursed_by IS NOT NULL
            AND rejected_by IS NULL AND disbursement_date IS NOT NULL
            AND settled_on IS NOT NULL AND rejection_reason IS NULL)
        -- Cancellation after approval retains approved_by for the audit trail.
        OR (status = 'REJECTED' AND disbursed_by IS NULL AND rejected_by IS NOT NULL
            AND disbursement_date IS NULL AND settled_on IS NULL
            AND rejection_reason IS NOT NULL AND CHAR_LENGTH(TRIM(rejection_reason)) > 0)
    ),
    CONSTRAINT chk_loans_dates CHECK (
        (disbursement_date IS NULL OR disbursement_date >= application_date)
        AND (settled_on IS NULL OR (disbursement_date IS NOT NULL AND settled_on >= disbursement_date))
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE installments (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    loan_id BIGINT NOT NULL,
    installment_number SMALLINT NOT NULL,
    due_date DATE NOT NULL,
    principal_due BIGINT NOT NULL,
    interest_due BIGINT NOT NULL,
    interest_waived BIGINT NOT NULL DEFAULT 0,
    waived_on DATE NULL,
    closed_on DATE NULL,
    UNIQUE KEY uq_loan_installment_number (loan_id, installment_number),
    CONSTRAINT fk_installments_loan FOREIGN KEY (loan_id) REFERENCES loans (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_installments_number CHECK (installment_number BETWEEN 1 AND 24),
    CONSTRAINT chk_installments_due CHECK (DAYOFMONTH(due_date) = 10),
    CONSTRAINT chk_installments_amounts CHECK (
        principal_due >= 0 AND interest_due >= 0
        AND interest_waived >= 0 AND interest_waived <= interest_due
    ),
    CONSTRAINT chk_installments_waiver CHECK (
        (interest_waived = 0 AND waived_on IS NULL)
        OR (interest_waived > 0 AND waived_on IS NOT NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE payments (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    loan_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    payment_type VARCHAR(20) COLLATE utf8mb4_0900_bin NOT NULL,
    payment_date DATE NOT NULL,
    amount BIGINT NOT NULL,
    request_key CHAR(32) COLLATE utf8mb4_0900_bin NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY uq_payments_request_key (request_key),
    KEY idx_payments_loan_date (loan_id, payment_date, id),
    CONSTRAINT fk_payments_loan FOREIGN KEY (loan_id) REFERENCES loans (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_payments_user FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_payments_type CHECK (payment_type IN ('REGULAR', 'EARLY_SETTLEMENT')),
    CONSTRAINT chk_payments_amount CHECK (amount > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE payment_allocations (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    payment_id BIGINT NOT NULL,
    installment_id BIGINT NULL,
    component_type VARCHAR(10) COLLATE utf8mb4_0900_bin NOT NULL,
    amount BIGINT NOT NULL,
    KEY idx_allocations_payment (payment_id),
    KEY idx_allocations_installment (installment_id),
    CONSTRAINT fk_allocations_payment FOREIGN KEY (payment_id) REFERENCES payments (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_allocations_installment FOREIGN KEY (installment_id) REFERENCES installments (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_allocations_component CHECK (component_type IN ('DENDA', 'JASA', 'POKOK', 'ADMIN')),
    CONSTRAINT chk_allocations_amount CHECK (amount > 0),
    CONSTRAINT chk_allocations_target CHECK (
        (component_type = 'ADMIN' AND installment_id IS NULL)
        OR (component_type IN ('DENDA', 'JASA', 'POKOK') AND installment_id IS NOT NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE loan_settlements (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    loan_id BIGINT NOT NULL,
    payment_id BIGINT NOT NULL,
    current_installment_id BIGINT NOT NULL,
    arrears_amount BIGINT NOT NULL,
    remaining_principal BIGINT NOT NULL,
    current_interest BIGINT NOT NULL,
    administration_fee BIGINT NOT NULL,
    waived_interest BIGINT NOT NULL,
    UNIQUE KEY uq_settlements_loan (loan_id),
    UNIQUE KEY uq_settlements_payment (payment_id),
    CONSTRAINT fk_settlements_loan FOREIGN KEY (loan_id) REFERENCES loans (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_settlements_payment FOREIGN KEY (payment_id) REFERENCES payments (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT fk_settlements_installment FOREIGN KEY (current_installment_id) REFERENCES installments (id)
        ON DELETE RESTRICT ON UPDATE RESTRICT,
    CONSTRAINT chk_settlements_amounts CHECK (
        arrears_amount >= 0 AND remaining_principal >= 0 AND current_interest >= 0
        AND administration_fee >= 0 AND waived_interest >= 0
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
