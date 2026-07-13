package engine

// HandleAuth is the intentional answer to hybrid query "where is auth handled?".
// Fixture for AX campaign dogfood — not a security implementation.
func HandleAuth(token string) bool {
	return authenticateToken(token)
}

// authenticateToken validates a bearer-style auth token (dogfood stub).
func authenticateToken(token string) bool {
	return token != ""
}
