package seed

type SeedMember struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
	Phone     string
	Address   string
}

type SeedTransaction struct {
	Amount      int64
	Description string
	Confirmed   bool
}

var Users = []SeedMember{
	{
		Email:     "jane.smith@example.com",
		Password:  "password123",
		FirstName: "Jane",
		LastName:  "Smith",
		Phone:     "+1234567892",
		Address:   "789 User Avenue, User Town",
	},
	{
		Email:     "bob.wilson@example.com",
		Password:  "password123",
		FirstName: "Bob",
		LastName:  "Wilson",
		Phone:     "+1234567893",
		Address:   "321 Cooperative Street, Coop City",
	},
	{
		Email:     "alice.brown@example.com",
		Password:  "password123",
		FirstName: "Alice",
		LastName:  "Brown",
		Phone:     "+1234567894",
		Address:   "654 Savings Road, Finance District",
	},
	{
		Email:     "john.doe@example.com",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
		Phone:     "+1234567891",
		Address:   "456 Member Lane, Member City",
	},
	{
		Email:     "sarah.johnson@example.com",
		Password:  "password123",
		FirstName: "Sarah",
		LastName:  "Johnson",
		Phone:     "+1234567895",
		Address:   "987 Cooperative Avenue, Finance City",
	},
}

var SavingsTransactions = []SeedTransaction{
	{
		Amount:      50000, // ₦500.00
		Description: "Initial membership deposit",
		Confirmed:   true,
	},
	{
		Amount:      25000, // ₦250.00
		Description: "Monthly savings contribution",
		Confirmed:   true,
	},
	{
		Amount:      30000, // ₦300.00
		Description: "Bonus savings deposit",
		Confirmed:   false, // Pending approval
	},
	{
		Amount:      15000, // ₦150.00
		Description: "Weekly savings contribution",
		Confirmed:   false, // Pending approval
	},
	{
		Amount:      40000, // ₦400.00
		Description: "End of month savings",
		Confirmed:   true,
	},
	{
		Amount:      20000, // ₦200.00
		Description: "Emergency fund contribution",
		Confirmed:   false, // Pending approval
	},
}
