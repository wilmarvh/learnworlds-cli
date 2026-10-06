package cli

// ep declares one API endpoint as a CLI command. Positional args fill the
// {path} params in order, then the body keys in ArgBody.
type ep struct {
	Group   string // "users" or "community spaces"
	Name    string // "list"
	Args    string // "<user> <product>"
	Short   string
	Method  string
	Path    string
	Query   []string // "name", "name:int", "name:time", "name:bool"
	Body    []bf
	ArgBody []string
	Paged   bool // has ?page=; gets --page / --all
	Limit   bool // has ?items_per_page; gets --limit
	Cols    []string
	Next    []Crumb
}

// bf maps a typed flag into a JSON body key.
type bf struct {
	Flag, Key, Kind, Usage, Def string // Kind: str, num, int, bool, list
}

func next(pairs ...string) []Crumb {
	var c []Crumb
	for i := 0; i+1 < len(pairs); i += 2 {
		c = append(c, Crumb{pairs[i], pairs[i+1]})
	}
	return c
}

var (
	userCols    = []string{"id", "email", "username", "role", "created"}
	paymentCols = []string{"id", "user_id", "product.name", "price", "paid_at"}
	courseBody  = []bf{
		{"title", "title", "str", "course title", ""},
		{"access", "access", "str", "paid | free | coming_soon | private | draft", ""},
		{"price", "price", "num", "course price", ""},
		{"description", "description", "str", "course description", ""},
		{"categories", "categories", "list", "comma-separated categories", ""},
		{"label", "label", "str", "course label", ""},
		{"drip-feed", "dripFeed", "str", "days | date | none", ""},
	}
	userBody = []bf{
		{"email", "email", "str", "email address", ""},
		{"username", "username", "str", "username", ""},
		{"tags", "tags", "list", "comma-separated tags", ""},
		{"admin", "is_admin", "bool", "make the user an admin", ""},
		{"marketing", "subscribed_for_marketing_emails", "bool", "subscribe to marketing emails", ""},
	}
	seatBody = []bf{
		{"title", "title", "str", "title", ""},
		{"description", "description", "str", "description", ""},
		{"seats", "number_of_seats", "int", "number of seats", ""},
		{"max-users", "max_number_of_users", "int", "max number of users", ""},
		{"tags", "tags", "list", "comma-separated tags", ""},
		{"access", "access", "str", "access", ""},
	}
	groupBody = []bf{
		{"title", "title", "str", "title", ""},
		{"description", "description", "str", "description", ""},
		{"max-users", "max_number_of_users", "int", "max number of users", ""},
		{"enroll-users", "enroll_users_on_courses", "bool", "enroll members on the group's courses", ""},
		{"tags", "tags", "list", "comma-separated tags", ""},
	}
	spaceBody = []bf{
		{"title", "title", "str", "space title", ""},
		{"description", "description", "str", "description", ""},
		{"access", "access", "str", "public | private | standalone", ""},
		{"collection", "collectionId", "str", "collection id", ""},
		{"hidden", "hidden_from_community", "bool", "hide from community", ""},
	}
)

var endpoints = []ep{
	// Courses
	{Group: "courses", Name: "list", Short: "List courses", Method: "GET", Path: "/v2/courses", Query: []string{"categories", "access"}, Paged: true,
		Cols: []string{"id", "title", "access", "final_price", "created"}, Next: next("show", "lw courses show <course>", "users", "lw courses users <course>")},
	{Group: "courses", Name: "show", Args: "<course>", Short: "Show a course", Method: "GET", Path: "/v2/courses/{id}",
		Next: next("contents", "lw courses contents <course>", "users", "lw courses users <course>", "analytics", "lw courses analytics <course>")},
	{Group: "courses", Name: "create", Short: "Create a course", Method: "POST", Path: "/v2/courses",
		Body: append([]bf{{"title-id", "titleId", "str", "course id/slug (required)", ""}}, courseBody...)},
	{Group: "courses", Name: "update", Args: "<course>", Short: "Update a course", Method: "PUT", Path: "/v2/courses/{id}", Body: courseBody},
	{Group: "courses", Name: "users", Args: "<course>", Short: "List users enrolled in a course", Method: "GET", Path: "/v2/courses/{id}/users", Query: []string{"include_suspended:bool"}, Paged: true, Limit: true, Cols: userCols},
	{Group: "courses", Name: "contents", Args: "<course>", Short: "Show sections and learning units of a course", Method: "GET", Path: "/v2/courses/{id}/contents"},
	{Group: "courses", Name: "grades", Args: "<course>", Short: "List grades in a course", Method: "GET", Path: "/v2/courses/{id}/grades", Query: []string{"users", "learningUnits", "sort", "order"}, Paged: true, Limit: true},
	{Group: "courses", Name: "analytics", Args: "<course>", Short: "Show analytics for a course", Method: "GET", Path: "/v2/courses/{id}/analytics"},
	{Group: "courses", Name: "unit-analytics", Args: "<course> <unit>", Short: "Show analytics for a learning activity", Method: "GET", Path: "/v2/courses/{id}/units/{uid}/analytics"},
	{Group: "courses", Name: "add-section", Args: "<course>", Short: "Create a course section (body via --data)", Method: "POST", Path: "/v2/courses/{id}/sections"},

	// Products
	{Group: "bundles", Name: "list", Short: "List bundles", Method: "GET", Path: "/v2/bundles", Paged: true, Next: next("show", "lw bundles show <bundle>")},
	{Group: "bundles", Name: "show", Args: "<bundle>", Short: "Show a bundle", Method: "GET", Path: "/v2/bundles/{id}"},
	{Group: "plans", Name: "list", Short: "List subscription plans", Method: "GET", Path: "/v2/subscription-plans", Paged: true, Next: next("show", "lw plans show <plan>")},
	{Group: "plans", Name: "show", Args: "<plan>", Short: "Show a subscription plan", Method: "GET", Path: "/v2/subscription-plans/{id}"},
	{Group: "subscriptions", Name: "list", Short: "List user subscriptions", Method: "GET", Path: "/v2/user-subscriptions", Query: []string{"user_id", "status"}, Paged: true,
		Cols: []string{"user_id", "email", "plan_id", "status", "expires_at"}},
	{Group: "installments", Name: "list", Short: "List active installments", Method: "GET", Path: "/v2/installments/active", Query: []string{"product_id", "product_type", "user_id"}, Paged: true},
	{Group: "events", Name: "list", Short: "List school calendar events (live sessions, drip feed, assignments)", Method: "GET", Path: "/v2/school/events", Query: []string{"event_type"},
		Cols: []string{"title", "type", "productId", "startDate"}},

	// Users
	{Group: "users", Name: "list", Short: "List users", Method: "GET", Path: "/v2/users",
		Query: []string{"status", "role", "tags", "registration_after:time", "registration_before:time", "include_suspended:bool"}, Paged: true, Limit: true, Cols: userCols,
		Next: next("show", "lw users show <user>", "courses", "lw users courses <user>")},
	{Group: "users", Name: "show", Args: "<user>", Short: "Show a user (id or email)", Method: "GET", Path: "/v2/users/{id}", Query: []string{"include_suspended:bool"},
		Next: next("courses", "lw users courses <user>", "progress", "lw users progress <user>", "enroll", "lw users enroll <user> <course>")},
	{Group: "users", Name: "create", Short: "Create a user", Method: "POST", Path: "/v2/users",
		Body: append(userBody, bf{"password", "password", "str", "password", ""}, bf{"send-email", "send_registration_email", "bool", "send registration email", ""}),
		Next: next("enroll", "lw users enroll <user> <course>")},
	{Group: "users", Name: "update", Args: "<user>", Short: "Update a user", Method: "PUT", Path: "/v2/users/{id}", Body: userBody},
	{Group: "users", Name: "courses", Args: "<user>", Short: "List a user's course enrollments", Method: "GET", Path: "/v2/users/{id}/courses", Paged: true,
		Cols: []string{"course.id", "course.title", "created", "expires"}, Next: next("progress", "lw users course-progress <user> <course>")},
	{Group: "users", Name: "products", Args: "<user>", Short: "List a user's products", Method: "GET", Path: "/v2/users/{id}/products"},
	{Group: "users", Name: "progress", Args: "<user>", Short: "Show a user's progress across courses", Method: "GET", Path: "/v2/users/{id}/progress", Paged: true, Limit: true,
		Cols: []string{"course_id", "status", "progress_rate", "average_score_rate", "completed_at"}},
	{Group: "users", Name: "course-progress", Args: "<user> <course>", Short: "Show a user's progress in a course", Method: "GET", Path: "/v2/users/{id}/courses/{cid}/progress"},
	{Group: "users", Name: "enroll", Args: "<user> <product>", Short: "Enroll a user in a course, bundle or subscription", Method: "POST", Path: "/v2/users/{id}/enrollment",
		ArgBody: []string{"productId"}, Body: []bf{
			{"type", "productType", "str", "course | bundle | subscription", "course"},
			{"price", "price", "num", "price recorded for the enrollment", "0"},
			{"justification", "justification", "str", "note shown in the enrollment log", ""},
			{"notify", "send_enrollment_email", "bool", "send the enrollment email", ""},
			{"duration", "duration", "int", "subscription duration", ""},
			{"duration-type", "duration_type", "str", "months | weeks | days (subscriptions)", ""},
		}, Next: next("verify", "lw users courses <user>")},
	{Group: "users", Name: "unenroll", Args: "<user> <product>", Short: "Unenroll a user from a product", Method: "DELETE", Path: "/v2/users/{id}/enrollment",
		ArgBody: []string{"productId"}, Body: []bf{{"type", "productType", "str", "course | bundle | subscription", "course"}}},
	{Group: "users", Name: "tag", Args: "<user>", Short: "Add (--add) or remove (--remove) user tags", Method: "PUT", Path: "/v2/users/{id}/tags"},
	{Group: "users", Name: "suspend", Args: "<user>", Short: "Suspend a user", Method: "PUT", Path: "/v2/users/{id}/suspend"},
	{Group: "users", Name: "unsuspend", Args: "<user>", Short: "Unsuspend a user", Method: "PUT", Path: "/v2/users/{id}/unsuspend"},
	{Group: "users", Name: "complete-course", Args: "<user> <course>", Short: "Mark a course (or units) complete for a user", Method: "POST", Path: "/v2/users/{id}/courses/{cid}/complete",
		Body: []bf{{"units", "units", "list", "unit ids (empty = whole course)", "[]"}, {"notify", "send_course_complete_email", "bool", "send completion email", "false"}}},
	{Group: "users", Name: "reset-course", Args: "<user> <course>", Short: "Reset a user's progress in a course (or units)", Method: "POST", Path: "/v2/users/{id}/courses/{cid}/reset",
		Body: []bf{{"units", "units", "list", "unit ids (empty = whole course)", "[]"}}},
	{Group: "users", Name: "segments", Short: "List user segments", Method: "GET", Path: "/v2/users/segments", Next: next("users", "lw users by-segment --segment-id <segment>")},
	{Group: "users", Name: "by-segment", Short: "List users in a segment", Method: "GET", Path: "/v2/users/by-segment", Query: []string{"segment_id", "include_suspended:bool"}, Paged: true, Cols: userCols},
	{Group: "users", Name: "by-product", Short: "List users of a product", Method: "GET", Path: "/v2/users/by-product", Query: []string{"product_id", "product_type", "include_suspended:bool"}, Paged: true, Cols: userCols},
	{Group: "users", Name: "seats", Args: "<user>", Short: "List seat offerings a user belongs to", Method: "GET", Path: "/v2/users/{id}/seats"},
	{Group: "users", Name: "groups", Args: "<user>", Short: "List user groups a user belongs to", Method: "GET", Path: "/v2/users/{id}/user-groups"},
	{Group: "users", Name: "role", Args: "<user>", Short: "Show a user's role", Method: "GET", Path: "/v2/users/{id}/user-role"},
	{Group: "users", Name: "set-role", Args: "<user>", Short: "Change a user's role", Method: "PUT", Path: "/v2/users/{id}/user-role",
		Body: []bf{{"role", "role_id", "str", "role id (see: lw roles list)", ""}, {"courses", "assigned_courses", "list", "assigned course ids", ""},
			{"seats", "assigned_seat_offering_ids", "list", "assigned seat offering ids", ""}, {"groups", "assigned_user_group_ids", "list", "assigned user group ids", ""},
			{"segment", "assigned_segment_id", "str", "assigned segment id", ""}}},

	// Commerce
	{Group: "payments", Name: "list", Short: "List payments", Method: "GET", Path: "/v2/payments",
		Query: []string{"user_id", "product_id", "product_type", "affiliate_id", "created_after:time", "created_before:time"}, Paged: true, Limit: true, Cols: paymentCols,
		Next: next("show", "lw payments show <payment>", "invoice", "lw payments invoice <payment>")},
	{Group: "payments", Name: "show", Args: "<payment>", Short: "Show a payment", Method: "GET", Path: "/v2/payments/{id}"},
	{Group: "payments", Name: "invoice", Args: "<payment>", Short: "Get an invoice link for a payment", Method: "GET", Path: "/v2/payments/{id}/invoice-link"},
	{Group: "leads", Name: "list", Short: "List leads", Method: "GET", Path: "/v2/leads", Paged: true, Cols: []string{"email", "first_name", "last_name", "user_id", "created"}},
	{Group: "promotions", Name: "list", Short: "List promotions", Method: "GET", Path: "/v2/promotions", Paged: true, Next: next("coupons", "lw promotions coupons <promotion>")},
	{Group: "promotions", Name: "show", Args: "<promotion>", Short: "Show a promotion", Method: "GET", Path: "/v2/promotions/{id}"},
	{Group: "promotions", Name: "create", Short: "Create a promotion (products via --data)", Method: "POST", Path: "/v2/promotions",
		Body: []bf{{"name", "name", "str", "promotion name", ""}, {"type", "type", "str", "percentage | fixed | flat", ""}, {"value", "value", "num", "discount value", ""},
			{"applies-to", "applies_to_all", "list", "bundles | courses | courses_bundles | none", ""}}},
	{Group: "promotions", Name: "coupons", Args: "<promotion>", Short: "List coupons of a promotion", Method: "GET", Path: "/v2/promotions/{pid}/coupons", Cols: []string{"code", "quantity", "times_used", "expires"}},
	{Group: "promotions", Name: "add-coupon", Args: "<promotion>", Short: "Create a coupon", Method: "POST", Path: "/v2/promotions/{pid}/coupons",
		Body: []bf{{"code", "code", "str", "coupon code", ""}, {"quantity", "quantity", "int", "number of uses", ""}, {"expires", "expires", "str", "expiry date YYYY-MM-DD", ""}}},
	{Group: "promotions", Name: "bulk-coupons", Args: "<promotion>", Short: "Create coupons in bulk", Method: "POST", Path: "/v2/promotions/{id}/coupons-bulk",
		Body: []bf{{"prefix", "prefix", "str", "code prefix", ""}, {"quantity", "quantity", "int", "number of coupons", ""}, {"expires", "expires", "str", "expiry date YYYY-MM-DD", ""}}},
	{Group: "promotions", Name: "coupon-usage", Args: "<promotion> <coupon>", Short: "Show coupon usage", Method: "GET", Path: "/v2/promotions/{pid}/coupons/{cid}/usage", Paged: true},
	{Group: "affiliates", Name: "list", Short: "List affiliates", Method: "GET", Path: "/v2/affiliates", Paged: true, Cols: []string{"id", "email", "code", "clicks", "sales"}},
	{Group: "affiliates", Name: "create", Args: "<user>", Short: "Make a user an affiliate", Method: "POST", Path: "/v2/affiliates/{id}",
		Body: []bf{{"commission", "commission_percentage", "num", "commission percentage", ""}, {"payment-method", "paymentMethod", "str", "paypal | bank_transfer | other | none", ""},
			{"payment-notes", "paymentNotes", "str", "payment notes", ""}}},
	{Group: "affiliates", Name: "leads", Args: "<affiliate>", Short: "List an affiliate's leads", Method: "GET", Path: "/v2/affiliates/{id}/leads", Paged: true, Cols: userCols},
	{Group: "affiliates", Name: "customers", Args: "<affiliate>", Short: "List an affiliate's customers", Method: "GET", Path: "/v2/affiliates/{id}/customers", Paged: true, Cols: userCols},
	{Group: "affiliates", Name: "payments", Args: "<affiliate>", Short: "List an affiliate's payments", Method: "GET", Path: "/v2/affiliates/{id}/payments", Paged: true, Cols: paymentCols},
	{Group: "affiliates", Name: "payouts", Args: "<affiliate> <upcoming|due|completed>", Short: "List an affiliate's payouts", Method: "GET", Path: "/v2/affiliates/{id}/payouts/{status}"},

	// Learning records
	{Group: "certificates", Name: "list", Short: "List certificates", Method: "GET", Path: "/v2/certificates", Query: []string{"course_id", "user_id"}, Paged: true,
		Cols: []string{"id", "title", "user.email", "course_id", "issued"}},
	{Group: "certificates", Name: "update", Args: "<certificate>", Short: "Reissue a certificate (body via --data)", Method: "PUT", Path: "/v2/certificates/{id}"},
	{Group: "certificates", Name: "delete", Args: "<certificate>", Short: "Revoke a certificate", Method: "DELETE", Path: "/v2/certificates/{id}"},
	{Group: "logs", Name: "list", Short: "List event logs", Method: "GET", Path: "/v2/event-logs",
		Query: []string{"user_id", "activity", "created_after:time", "created_before:time", "sort"}, Paged: true, Cols: []string{"created", "activity", "user.email", "description"}},
	{Group: "assessments", Name: "responses", Args: "<assessment>", Short: "List assessment responses", Method: "GET", Path: "/v2/assessments/{id}/responses", Query: []string{"users"}, Paged: true, Limit: true,
		Cols: []string{"id", "email", "grade", "passed", "submittedTimestamp"}},
	{Group: "assessments", Name: "review", Args: "<score>", Short: "Review an assessment submission", Method: "POST", Path: "/v2/assessments/scores/{id}/review",
		Body: []bf{{"feedback", "generalFeedback", "str", "general feedback", ""}}},
	{Group: "forms", Name: "responses", Args: "<form>", Short: "List form responses", Method: "GET", Path: "/v2/forms/{id}/responses", Query: []string{"users"}, Paged: true, Limit: true,
		Cols: []string{"id", "email", "submittedTimestamp"}},
	{Group: "async", Name: "show", Args: "<action>", Short: "Show progress of an asynchronous action", Method: "GET", Path: "/v2/async-actions/{id}"},

	// Seats & groups
	{Group: "seats", Name: "list", Short: "List seat offerings", Method: "GET", Path: "/v2/seats", Paged: true, Cols: []string{"id", "title", "number_of_seats", "available_seats", "created"}},
	{Group: "seats", Name: "show", Args: "<seat>", Short: "Show a seat offering", Method: "GET", Path: "/v2/seats/{id}"},
	{Group: "seats", Name: "create", Short: "Create a seat offering (products via --data)", Method: "POST", Path: "/v2/seats", Body: seatBody},
	{Group: "seats", Name: "update", Args: "<seat>", Short: "Update a seat offering", Method: "PUT", Path: "/v2/seats/{id}", Body: seatBody},
	{Group: "seats", Name: "delete", Args: "<seat>", Short: "Delete a seat offering", Method: "DELETE", Path: "/v2/seats/{id}"},
	{Group: "seats", Name: "users", Args: "<seat>", Short: "List users of a seat offering", Method: "GET", Path: "/v2/seats/{id}/users", Paged: true},
	{Group: "seats", Name: "add-user", Args: "<seat> <user>", Short: "Add a user to a seat offering", Method: "POST", Path: "/v2/seats/{id}/users/{uid}",
		Body: []bf{{"active", "add_to_active_seat", "bool", "add to an active seat", ""}}},
	{Group: "seats", Name: "remove-user", Args: "<seat> <user>", Short: "Remove a user from a seat", Method: "DELETE", Path: "/v2/seats/{id}/users/{uid}",
		Body: []bf{{"from-offering", "remove_from_seat_offering", "bool", "also remove from the seat offering", ""}}},
	{Group: "groups", Name: "list", Short: "List user groups", Method: "GET", Path: "/v2/user_groups", Paged: true, Cols: []string{"id", "title", "max_number_of_users", "created"}},
	{Group: "groups", Name: "show", Args: "<group>", Short: "Show a user group", Method: "GET", Path: "/v2/user_groups/{id}"},
	{Group: "groups", Name: "create", Short: "Create a user group (products via --data)", Method: "POST", Path: "/v2/user_groups", Body: groupBody},
	{Group: "groups", Name: "update", Args: "<group>", Short: "Update a user group", Method: "PUT", Path: "/v2/user_groups/{id}", Body: groupBody},
	{Group: "groups", Name: "delete", Args: "<group>", Short: "Delete a user group", Method: "DELETE", Path: "/v2/user_groups/{id}"},
	{Group: "groups", Name: "users", Args: "<group>", Short: "List users of a user group", Method: "GET", Path: "/v2/user_groups/{id}/users", Paged: true, Cols: userCols},
	{Group: "groups", Name: "add-user", Args: "<group> <user>", Short: "Add a user to a user group", Method: "POST", Path: "/v2/user_groups/{id}/users/{uid}"},
	{Group: "groups", Name: "remove-user", Args: "<group> <user>", Short: "Remove a user from a user group", Method: "DELETE", Path: "/v2/user_groups/{id}/users/{uid}"},
	{Group: "roles", Name: "list", Short: "List user roles", Method: "GET", Path: "/v2/user-roles", Query: []string{"role_id", "access_level"}},

	// Community
	{Group: "community collections", Name: "list", Short: "List community collections", Method: "GET", Path: "/v2/community/collections"},
	{Group: "community spaces", Name: "list", Short: "List community spaces", Method: "GET", Path: "/v2/community/spaces", Query: []string{"access", "collectionId", "usages"}, Paged: true, Limit: true},
	{Group: "community spaces", Name: "show", Args: "<space>", Short: "Show a community space", Method: "GET", Path: "/v2/community/spaces/{id}"},
	{Group: "community spaces", Name: "create", Short: "Create a community space", Method: "POST", Path: "/v2/community/spaces", Body: spaceBody},
	{Group: "community spaces", Name: "update", Args: "<space>", Short: "Update a community space", Method: "PUT", Path: "/v2/community/spaces/{id}", Body: spaceBody},
	{Group: "community spaces", Name: "delete", Args: "<space>", Short: "Delete a community space", Method: "DELETE", Path: "/v2/community/spaces/{id}"},
	{Group: "community spaces", Name: "users", Args: "<space>", Short: "List users of a community space", Method: "GET", Path: "/v2/community/spaces/{id}/users", Paged: true, Limit: true},
	{Group: "community spaces", Name: "add-users", Args: "<space>", Short: "Add or invite users to a community space", Method: "POST", Path: "/v2/community/spaces/{id}/users",
		Body: []bf{{"users", "uids", "list", "comma-separated user ids", ""}}},
	{Group: "community spaces", Name: "remove-user", Args: "<space> <user>", Short: "Remove a user from a community space", Method: "DELETE", Path: "/v2/community/spaces/{id}/users/{uid}"},
	{Group: "community posts", Name: "list", Short: "List community posts", Method: "GET", Path: "/v2/community/posts", Query: []string{"user_id", "space_id", "course_id", "mentions"}, Paged: true, Limit: true,
		Cols: []string{"id", "user.username", "text", "created"}},
	{Group: "community posts", Name: "show", Args: "<post>", Short: "Show a community post", Method: "GET", Path: "/v2/community/posts/{id}"},
}

var groupShort = map[string]string{
	"courses": "Courses, contents, grades and analytics", "bundles": "Bundles (learning programs)", "plans": "Subscription plans",
	"subscriptions": "User subscriptions", "installments": "Installment plans", "events": "School calendar events",
	"users": "Users, enrollments, progress and roles", "payments": "Payments and invoices", "leads": "Leads",
	"promotions": "Promotions and coupons", "affiliates": "Affiliates, payouts and referrals", "certificates": "Certificates",
	"logs": "Event logs", "assessments": "Assessment responses and reviews", "forms": "Form responses",
	"async": "Asynchronous actions", "seats": "Multiple-seat offerings", "groups": "User groups", "roles": "User roles",
	"community": "Community collections, spaces and posts", "community collections": "Community collections",
	"community spaces": "Community spaces", "community posts": "Community posts",
}
