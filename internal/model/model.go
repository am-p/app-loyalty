package model

import "time"

type RegisterCustomerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}
type RegisterDemoMerchantRequest struct {
	Email         string  `json:"email"`
	Password      string  `json:"password"`
	OwnerName     string  `json:"owner_name"`
	BrandName     string  `json:"brand_name"`
	BranchName    string  `json:"branch_name"`
	BranchAddress *string `json:"branch_address,omitempty"`
	ProgramType   string  `json:"program_type"`
	AccessCode    string  `json:"access_code"`
}
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}
type GoogleAuthRequest struct {
	IDToken string `json:"id_token"`
}

type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	LastName      *string   `json:"apellido,omitempty"`
	Alias         *string   `json:"alias,omitempty"`
	PhotoURL      *string   `json:"foto_url,omitempty"`
	AccountType   string    `json:"account_type"`
	Active        bool      `json:"active"`
	EmailVerified bool      `json:"email_verified"`
	AuthVersion   int       `json:"auth_version"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
}
type UpdateAccountRequest struct {
	Name     string  `json:"nombre"`
	LastName *string `json:"apellido,omitempty"`
	Alias    *string `json:"alias,omitempty"`
	PhotoURL *string `json:"foto_url,omitempty"`
}
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}
type AuthData struct {
	Session Session `json:"session"`
	User    User    `json:"user"`
}
type RegisterCustomerData struct {
	User                 User     `json:"user"`
	Session              *Session `json:"session,omitempty"`
	VerificationRequired bool     `json:"verification_required"`
}
type EmailRequest struct {
	Email string `json:"email"`
}
type TokenRequest struct {
	Token string `json:"token"`
}
type PasswordResetConfirmRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}
type EmailMessage struct {
	Kind, To, Subject, Text, HTML string
}
type OutboxEmail struct {
	ID, To, Subject, Text, HTML string
	Attempts                    int
}
type Membership struct {
	BrandID   int64   `json:"brand_id"`
	BrandName string  `json:"brand_name"`
	Role      string  `json:"role"`
	BranchIDs []int64 `json:"branch_ids"`
}
type CurrentUser struct {
	User               User         `json:"user"`
	Memberships        []Membership `json:"memberships"`
	OnboardingComplete bool         `json:"onboarding_complete"`
}
type Customer struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	QRToken string `json:"qr_token"`
}

type DemoAccess struct {
	Kind            string    `json:"kind"`
	PriceMinor      int64     `json:"price_minor"`
	Currency        string    `json:"currency"`
	AutomaticCharge bool      `json:"automatic_charge"`
	Active          bool      `json:"active"`
	StartedAt       time.Time `json:"started_at"`
}
type Branch struct {
	ID      int64   `json:"id"`
	BrandID int64   `json:"brand_id"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
	Active  bool    `json:"active"`
}
type Program struct {
	ID                    int64  `json:"id"`
	BrandID               int64  `json:"brand_id"`
	Type                  string `json:"type"`
	StampsPerAccumulation *int64 `json:"stamps_per_accumulation,omitempty"`
	Active                bool   `json:"active"`
}
type Benefit struct {
	ID             int64  `json:"id"`
	ProgramID      int64  `json:"program_id"`
	Name           string `json:"name"`
	RequiredStamps *int64 `json:"required_stamps,omitempty"`
	RequiredPoints *int64 `json:"required_points,omitempty"`
	Active         bool   `json:"active"`
	Version        int    `json:"version"`
}
type CreateBenefitRequest struct {
	Name        string `json:"name"`
	Requirement int64  `json:"requirement"`
}
type MerchantContext struct {
	BrandID    int64      `json:"brand_id"`
	BrandName  string     `json:"brand_name"`
	Role       string     `json:"role"`
	Branch     Branch     `json:"branch"`
	Program    Program    `json:"program"`
	Benefit    *Benefit   `json:"benefit,omitempty"`
	Benefits   []Benefit  `json:"benefits"`
	DemoAccess DemoAccess `json:"demo_access"`
}
type DemoMerchantData struct {
	Session              *Session        `json:"session,omitempty"`
	User                 User            `json:"user"`
	Merchant             MerchantContext `json:"merchant"`
	OnboardingComplete   bool            `json:"onboarding_complete"`
	VerificationRequired bool            `json:"verification_required"`
}

type Card struct {
	ID            int64   `json:"id"`
	BrandID       int64   `json:"brand_id"`
	BrandName     string  `json:"brand_name"`
	ProgramType   string  `json:"program_type"`
	BalanceStamps int64   `json:"balance_stamps"`
	BalancePoints int64   `json:"balance_points"`
	Benefit       Benefit `json:"benefit"`
}
type BrandCustomer struct {
	CustomerID     int64      `json:"customer_id"`
	CardID         int64      `json:"card_id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	ProgramType    string     `json:"program_type"`
	BalanceStamps  int64      `json:"balance_stamps"`
	BalancePoints  int64      `json:"balance_points"`
	MovementsCount int64      `json:"movements_count"`
	LastMovementAt *time.Time `json:"last_movement_at"`
	JoinedAt       time.Time  `json:"joined_at"`
}
type BrandMetricsSummary struct {
	ActiveCustomers     int64      `json:"active_customers"`
	ProgramType         string     `json:"program_type"`
	CurrentStampBalance int64      `json:"current_stamp_balance"`
	CurrentPointBalance int64      `json:"current_point_balance"`
	Accumulations       int64      `json:"accumulations"`
	Redemptions         int64      `json:"redemptions"`
	StampsIssued        int64      `json:"stamps_issued"`
	StampsRedeemed      int64      `json:"stamps_redeemed"`
	PointsIssued        int64      `json:"points_issued"`
	PointsRedeemed      int64      `json:"points_redeemed"`
	LastMovementAt      *time.Time `json:"last_movement_at"`
}
type MovementPreviewRequest struct {
	Operation    string `json:"operation"`
	QRToken      string `json:"qr_token,omitempty"`
	CustomerCode string `json:"customer_code,omitempty"`
	BranchID     int64  `json:"branch_id"`
	BenefitID    *int64 `json:"benefit_id,omitempty"`
	PointsAmount *int64 `json:"cantidad_puntos,omitempty"`
}
type ConfirmAccumulationRequest struct {
	PreviewID    string `json:"preview_id"`
	QRToken      string `json:"qr_token,omitempty"`
	CustomerCode string `json:"customer_code,omitempty"`
	BranchID     int64  `json:"branch_id"`
}
type ConfirmRedemptionRequest struct {
	PreviewID    string `json:"preview_id"`
	QRToken      string `json:"qr_token,omitempty"`
	CustomerCode string `json:"customer_code,omitempty"`
	BranchID     int64  `json:"branch_id"`
	BenefitID    int64  `json:"benefit_id"`
}
type PreviewCustomer struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Preview struct {
	ID            string          `json:"id"`
	ExpiresAt     time.Time       `json:"expires_at"`
	Operation     string          `json:"operation"`
	ProgramType   string          `json:"program_type"`
	Customer      PreviewCustomer `json:"customer"`
	CardID        int64           `json:"card_id"`
	BalanceBefore int64           `json:"balance_before"`
	Amount        int64           `json:"amount"`
	BalanceAfter  int64           `json:"balance_after"`
	Benefit       *Benefit        `json:"benefit,omitempty"`
}
type Movement struct {
	ID                            int64     `json:"id"`
	OperationID                   string    `json:"operation_id"`
	CardID                        int64     `json:"card_id"`
	BrandID                       int64     `json:"brand_id,omitempty"`
	BrandName                     string    `json:"brand_name,omitempty"`
	BranchID                      int64     `json:"branch_id"`
	BranchName                    string    `json:"branch_name,omitempty"`
	Operation                     string    `json:"operation"`
	ProgramType                   string    `json:"program_type"`
	ProgramIDSnapshot             int64     `json:"program_id_snapshot"`
	Direction                     string    `json:"direction"`
	Amount                        int64     `json:"amount"`
	BalanceBefore                 int64     `json:"balance_before"`
	BalanceAfter                  int64     `json:"balance_after"`
	BenefitNameSnapshot           *string   `json:"benefit_name_snapshot,omitempty"`
	BenefitRequiredStampsSnapshot *int64    `json:"benefit_required_stamps_snapshot,omitempty"`
	BenefitRequiredPointsSnapshot *int64    `json:"benefit_required_points_snapshot,omitempty"`
	OccurredAt                    time.Time `json:"occurred_at"`
}

type LegacyRegisterRequest struct {
	Name     string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type LegacyUser struct {
	ID           int64   `json:"id_usuario"`
	Email        string  `json:"email"`
	Name         string  `json:"nombre"`
	Role         string  `json:"rol"`
	Subscription *string `json:"suscripcion"`
}
type LegacyAuth struct {
	Token string     `json:"token"`
	User  LegacyUser `json:"usuario"`
}
