package domain

import "time"

type MemberStatus string

const (
	MemberActive MemberStatus = "ACTIVE"
	MemberExited MemberStatus = "EXITED"
)

type Member struct {
	ID           int64
	MemberNumber string
	Name         string
	Address      string
	Phone        string
	JoinDate     time.Time
	Status       MemberStatus
	ExitDate     *time.Time
	ExitUserID   *int64
	CreatedBy    int64
	CreatedAt    time.Time
}
