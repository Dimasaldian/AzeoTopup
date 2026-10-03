package admin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"topupku/handler"
	"topupku/middleware"
	"topupku/model"
	"topupku/store"
)

type GameAdminHandler struct {
	store *store.SQLiteStore
}

func NewGameAdminHandler(st *store.SQLiteStore) *GameAdminHandler {
	return &GameAdminHandler{store: st}
}

func (h *GameAdminHandler) GameList(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	games, err := h.store.GetAllGames(false)
	if err != nil {
		http.Error(w, "Gagal memuat game", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title":     "Kelola Game — Azeotopup Admin",
		"ActiveTab": "games",
		"AdminUser": adminUser,
		"Games":     games,
		"Message":   r.URL.Query().Get("msg"),
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/games.html",
	}, data)
}

func (h *GameAdminHandler) GameForm(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	var game *model.Game
	isEdit := false

	if idStr != "" && idStr != "new" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		g, err := h.store.GetGameByID(id)
		if err == nil {
			game = g
			isEdit = true
		}
	}

	if game == nil {
		game = &model.Game{
			IDLabel:       "User ID",
			IDPlaceholder: "Masukkan User ID",
			IsActive:      true,
		}
	}

	data := map[string]interface{}{
		"Title":     "Form Game — Azeotopup Admin",
		"ActiveTab": "games",
		"AdminUser": adminUser,
		"Game":      game,
		"IsEdit":    isEdit,
		"Error":     r.URL.Query().Get("error"),
	}

	handler.RenderTemplate(w, "template/admin/layout.html", []string{
		"template/admin/game_form.html",
	}, data)
}

func (h *GameAdminHandler) saveUploadedFile(r *http.Request, formKey string) (string, error) {
	file, header, err := r.FormFile(formKey)
	if err != nil {
		return "", nil // no file uploaded
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".svg" {
		return "", fmt.Errorf("format file tidak didukung: %s", ext)
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	destPath := filepath.Join("uploads", "games", filename)

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}

	return "/uploads/games/" + filename, nil
}

func (h *GameAdminHandler) GameCreate(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, "/admin/games/new?error=File+terlalu+besar", http.StatusSeeOther)
		return
	}

	code := strings.TrimSpace(strings.ToLower(r.FormValue("code")))
	name := strings.TrimSpace(r.FormValue("name"))
	brand := strings.TrimSpace(r.FormValue("brand"))

	if code == "" || name == "" || brand == "" {
		http.Redirect(w, r, "/admin/games/new?error=Kode,+nama,+dan+brand+wajib+diisi", http.StatusSeeOther)
		return
	}

	iconPath, _ := h.saveUploadedFile(r, "icon_file")
	if iconPath == "" {
		iconPath = strings.TrimSpace(r.FormValue("icon_url"))
	}
	if iconPath == "" {
		iconPath = "https://images.unsplash.com/photo-1542751371-adc38448a05e?w=128&q=80"
	}

	bannerPath, _ := h.saveUploadedFile(r, "banner_file")
	if bannerPath == "" {
		bannerPath = strings.TrimSpace(r.FormValue("banner_url"))
	}

	game := &model.Game{
		Code:            code,
		Name:            name,
		Brand:           brand,
		Description:     strings.TrimSpace(r.FormValue("description")),
		IconPath:        iconPath,
		BannerPath:      bannerPath,
		IDLabel:         strings.TrimSpace(r.FormValue("id_label")),
		IDPlaceholder:   strings.TrimSpace(r.FormValue("id_placeholder")),
		IDHelpText:      strings.TrimSpace(r.FormValue("id_help_text")),
		ID2Label:        strings.TrimSpace(r.FormValue("id2_label")),
		ID2Placeholder:  strings.TrimSpace(r.FormValue("id2_placeholder")),
		IDFormatRegex:   strings.TrimSpace(r.FormValue("id_format_regex")),
		InstructionText: strings.TrimSpace(r.FormValue("instruction_text")),
		IsActive:        r.FormValue("is_active") == "1" || r.FormValue("is_active") == "on",
	}

	id, err := h.store.CreateGame(game)
	if err != nil {
		http.Redirect(w, r, "/admin/games/new?error="+err.Error(), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "game.create", "game", fmt.Sprintf("%d", id), "", fmt.Sprintf("Created game %s (%s)", name, code))

	http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/products?msg=Game+berhasil+dibuat!+Silakan+tambahkan+atau+import+produk.", id), http.StatusSeeOther)
}

func (h *GameAdminHandler) GameUpdate(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=ID+tidak+valid", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/edit?error=File+terlalu+besar", id), http.StatusSeeOther)
		return
	}

	existing, err := h.store.GetGameByID(id)
	if err != nil {
		http.Redirect(w, r, "/admin/games?error=Game+tidak+ditemukan", http.StatusSeeOther)
		return
	}

	iconPath, _ := h.saveUploadedFile(r, "icon_file")
	if iconPath == "" && r.FormValue("icon_url") != "" {
		iconPath = strings.TrimSpace(r.FormValue("icon_url"))
	}
	if iconPath == "" {
		iconPath = existing.IconPath
	}

	bannerPath, _ := h.saveUploadedFile(r, "banner_file")
	if bannerPath == "" && r.FormValue("banner_url") != "" {
		bannerPath = strings.TrimSpace(r.FormValue("banner_url"))
	}
	if bannerPath == "" {
		bannerPath = existing.BannerPath
	}

	existing.Code = strings.TrimSpace(strings.ToLower(r.FormValue("code")))
	existing.Name = strings.TrimSpace(r.FormValue("name"))
	existing.Brand = strings.TrimSpace(r.FormValue("brand"))
	existing.Description = strings.TrimSpace(r.FormValue("description"))
	existing.IconPath = iconPath
	existing.BannerPath = bannerPath
	existing.IDLabel = strings.TrimSpace(r.FormValue("id_label"))
	existing.IDPlaceholder = strings.TrimSpace(r.FormValue("id_placeholder"))
	existing.IDHelpText = strings.TrimSpace(r.FormValue("id_help_text"))
	existing.ID2Label = strings.TrimSpace(r.FormValue("id2_label"))
	existing.ID2Placeholder = strings.TrimSpace(r.FormValue("id2_placeholder"))
	existing.IDFormatRegex = strings.TrimSpace(r.FormValue("id_format_regex"))
	existing.InstructionText = strings.TrimSpace(r.FormValue("instruction_text"))
	existing.IsActive = r.FormValue("is_active") == "1" || r.FormValue("is_active") == "on"

	if err := h.store.UpdateGame(existing); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/admin/games/%d/edit?error=%s", id, err.Error()), http.StatusSeeOther)
		return
	}

	_ = h.store.CreateAuditLog(adminUser.ID, "game.update", "game", fmt.Sprintf("%d", id), "", fmt.Sprintf("Updated game %s", existing.Name))

	http.Redirect(w, r, "/admin/games?msg=Game+berhasil+diupdate!", http.StatusSeeOther)
}

func (h *GameAdminHandler) GameToggle(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	_ = h.store.ToggleGame(id)
	_ = h.store.CreateAuditLog(adminUser.ID, "game.toggle", "game", idStr, "", "Toggled game status")

	http.Redirect(w, r, "/admin/games?msg=Status+game+berhasil+diubah", http.StatusSeeOther)
}

func (h *GameAdminHandler) GameDelete(w http.ResponseWriter, r *http.Request) {
	adminUser := middleware.GetAdminUser(r)

	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	_ = h.store.DeleteGame(id)
	_ = h.store.CreateAuditLog(adminUser.ID, "game.delete", "game", idStr, "", "Deleted game")

	http.Redirect(w, r, "/admin/games?msg=Game+berhasil+dihapus", http.StatusSeeOther)
}

func (h *GameAdminHandler) GameReorder(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	dir := r.URL.Query().Get("dir")

	_ = h.store.ReorderGame(id, dir)
	http.Redirect(w, r, "/admin/games", http.StatusSeeOther)
}
