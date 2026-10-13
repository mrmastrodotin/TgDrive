package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"tgdrive/storage"
)

// HandleListFolders handles GET /api/folders — lists root folders, or children if ?parent_id= is set.
func (s *Server) HandleListFolders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	parentID := r.URL.Query().Get("parent_id")
	var parentIDPtr *string
	if parentID != "" {
		parentIDPtr = &parentID
	}

	folders, err := s.db.ListFolders(ctx, parentIDPtr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list folders"})
		return
	}

	files, err := s.db.ListFiles(ctx, parentIDPtr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list files"})
		return
	}

	if folders == nil {
		folders = []storage.Folder{}
	}
	if files == nil {
		files = []storage.File{}
	}

	var parentFolder *storage.Folder
	if parentIDPtr != nil {
		pf, err := s.db.GetFolder(ctx, *parentIDPtr)
		if err == nil && pf != nil {
			parentFolder = pf
		}
	}

	writeJSON(w, http.StatusOK, storage.FolderContents{
		Folder:  parentFolder,
		Folders: folders,
		Files:   files,
	})
}

// HandleCreateFolder handles POST /api/folders
func (s *Server) HandleCreateFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "folder name is required"})
		return
	}

	folder, err := s.db.CreateFolder(ctx, req.Name, req.ParentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create folder"})
		return
	}

	writeJSON(w, http.StatusCreated, folder)
}

// HandleDeleteFolder handles DELETE /api/folders/:id
func (s *Server) HandleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	folderID := extractPathParam(r.URL.Path, "folders", "")
	if folderID == "" {
		folderID = r.PathValue("id")
	}

	folder, err := s.db.GetFolder(ctx, folderID)
	if err != nil || folder == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "folder not found"})
		return
	}

	s.deleteAllFilesInFolder(ctx, folderID)

	if err := s.db.DeleteFolder(ctx, folderID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete folder"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleRenameFolder handles PUT /api/folders/:id
func (s *Server) HandleRenameFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	folderID := extractPathParam(r.URL.Path, "folders", "")
	if folderID == "" {
		folderID = r.PathValue("id")
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if err := s.db.RenameFolder(ctx, folderID, req.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to rename folder"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

// deleteAllFilesInFolder removes Telegram messages for all files in a folder and its subfolders.
func (s *Server) deleteAllFilesInFolder(ctx context.Context, folderID string) {
	files, err := s.db.ListFiles(ctx, &folderID)
	if err == nil {
		for _, f := range files {
			msgIDs, err := s.db.GetAllFileChunkTelegramMsgIDs(ctx, f.ID)
			if err != nil {
				continue
			}
			for _, msgID := range msgIDs {
				_ = s.tg.DeleteMessage(ctx, msgID)
			}
		}
	}

	// Recursively clean subfolders
	subFolders, err := s.db.ListFolders(ctx, &folderID)
	if err == nil {
		for _, sf := range subFolders {
			s.deleteAllFilesInFolder(ctx, sf.ID)
		}
	}
}
