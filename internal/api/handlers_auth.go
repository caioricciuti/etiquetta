package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/caioricciuti/etiquetta/internal/auth"
)

// CheckSetup returns whether initial setup is complete
func (h *Handlers) CheckSetup(w http.ResponseWriter, r *http.Request) {
	var count int
	if err := h.db.Conn().QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to check setup status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"setup_complete": count > 0,
	})
}

// Setup creates the initial admin user
func (h *Handlers) Setup(w http.ResponseWriter, r *http.Request) {
	// Check if setup is already complete
	var count int
	h.db.Conn().QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if count > 0 {
		writeError(w, http.StatusBadRequest, "Setup already complete")
		return
	}

	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if input.Email == "" || input.Password == "" {
		writeError(w, http.StatusBadRequest, "Email and password are required")
		return
	}

	if len(input.Password) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Create admin user
	id := auth.GenerateID()
	now := time.Now().UnixMilli()

	_, err = h.db.Conn().Exec(
		"INSERT INTO users (id, email, password_hash, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'admin', ?, ?)",
		id, input.Email, passwordHash, input.Name, now, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	// Mark setup as complete
	h.db.Conn().Exec(
		"UPDATE settings SET value = 'true', updated_at = ? WHERE key = 'setup_complete'",
		now,
	)

	// Generate token and set cookie
	user := &auth.User{
		ID:    id,
		Email: input.Email,
		Role:  "admin",
	}
	token, err := h.auth.GenerateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	h.auth.SetAuthCookie(w, token)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"id":    id,
			"email": input.Email,
			"name":  input.Name,
			"role":  "admin",
		},
	})
}

// Login authenticates a user
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Find user
	var user auth.User
	err := h.db.Conn().QueryRow(
		"SELECT id, email, password_hash, name, role FROM users WHERE email = ?",
		input.Email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role)

	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Verify password
	if !auth.VerifyPassword(input.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// Generate token
	token, err := h.auth.GenerateToken(&user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	h.auth.SetAuthCookie(w, token)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"role":  user.Role,
		},
	})
}

// Logout clears the auth cookie
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.ClearAuthCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// GetCurrentUser returns the current authenticated user
func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Get full user data
	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}

	err := h.db.Conn().QueryRow(
		"SELECT id, email, name, role FROM users WHERE id = ?",
		claims.UserID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role)

	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

// UpdateProfile lets the authenticated user update their own name
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now().UnixMilli()
	_, err := h.db.Conn().Exec(
		"UPDATE users SET name = ?, updated_at = ? WHERE id = ?",
		input.Name, now, claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	// Return updated user
	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	h.db.Conn().QueryRow(
		"SELECT id, email, name, role FROM users WHERE id = ?",
		claims.UserID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role)

	writeJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

// ChangePassword changes the current user's password
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetUserFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(input.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	// Verify current password
	var currentHash string
	err := h.db.Conn().QueryRow(
		"SELECT password_hash FROM users WHERE id = ?",
		claims.UserID,
	).Scan(&currentHash)

	if err != nil || !auth.VerifyPassword(input.CurrentPassword, currentHash) {
		writeError(w, http.StatusBadRequest, "Current password is incorrect")
		return
	}

	// Hash new password
	newHash, err := auth.HashPassword(input.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Update password
	_, err = h.db.Conn().Exec(
		"UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?",
		newHash, time.Now().UnixMilli(), claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update password")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
