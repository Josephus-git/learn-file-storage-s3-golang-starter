package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	//Set maxMemory to 10MB using a bit shift
	const maxMemory = 10 << 20 // 10MB

	// ParseMultipartForm parses a multipart/form-data POST request.
	// It's crucial to call this before accessing form fields or files.
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to parse multipart form data", err)
		return
	}

	// Use r.FormFile to get the file data and file headers. The key is "thumbnail".
	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		if err == http.ErrMissingFile {
			respondWithError(w, http.StatusBadRequest, "No 'thumbnail' file provided in the form", err)
		} else {
			respondWithError(w, http.StatusBadRequest, "Failed to get 'thumbnail' file from form", err)
		}
		return
	}
	defer file.Close() // Ensure the file is closed after processing
	// get the media type from form files header
	mediaType := fileHeader.Header.Get("Content-Type")
	if mediaType != "image/png" && mediaType != "image/jpg" {
		respondWithError(w, http.StatusBadRequest, "Content must be image file", nil)
		return
	}

	// Extract file extension from media type (e.g., "image/png" -> "png")
	exts, _ := mime.ExtensionsByType(mediaType)
	ext := ".jpg" // default
	if len(exts) > 0 {
		ext = exts[0]
	}

	uniqueFilePath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s%s", videoID.String(), ext))
	filePath, err := os.Create(uniqueFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create file path", err)
		return
	}

	_, err = io.Copy(filePath, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to copy file", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized video owner", err)
		return
	}
	thumbnail_url := fmt.Sprintf("http://localhost:%s/assets/%s%s", cfg.port, videoID, ext)
	video.ThumbnailURL = &thumbnail_url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		delete(videoThumbnails, videoID)
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
