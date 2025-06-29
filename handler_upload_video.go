package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMemory = 10 << 30 // 1 GB

	// Limit the request body size using http.Maxbytereader
	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	// parse the uploaded video from the form data
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// validate uploaded file to ensure its mp4
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	// save uploaded file to temp file on disk

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file on server", err)
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	if _, err = io.Copy(tempFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file", err)
		return
	}

	// reset the tempfiles file pointer to the beginning
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reseting pointer", err)
		return
	}

	// --- Generate a unique file key ---
	// <random-32-byte-hex>.ext format
	randomBytes := make([]byte, 16) // 16 bytes -> 32 hex characters
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "generating random bytes", err)
		return
	}

	// get aspect ratio prefix
	var prefix string
	// Reset file pointer before reading for aspect ratio
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Print(err)
		respondWithError(w, http.StatusInternalServerError, "Error resetting pointer for aspect ratio", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting aspect ratio", err)
		return
	}
	if aspectRatio == "16:9" {
		prefix = "landscape"
	} else if aspectRatio == "9:16" {
		prefix = "portrait"
	} else {
		prefix = aspectRatio
	}

	fileKey := getAssetPath(mediaType)
	fileKeyWithPrefix := getAssetPathWithPrefix(prefix, fileKey)

	// process the video for fast encoding
	processedFilePath, err := processVidoeForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error processing video for fast start", err)
		return
	}

	// IMPORTANT: Ensure the processed temporary file is deleted after use
	defer func() {
		if rErr := os.Remove(processedFilePath); rErr != nil {
			fmt.Printf("Error deleting processed temp file %s: %v\n", processedFilePath, rErr)
		} else {
			fmt.Printf("Deleted processed temp file: %s\n", processedFilePath)
		}
	}()

	// Open the processed video file for S3 upload
	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error opening processed file", err)
		return
	}
	defer processedFile.Close()

	// put the object into s3 using PutObject
	item := &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(fileKeyWithPrefix),
		Body:        processedFile,
		ContentType: aws.String(mediaType),
	}

	_, err = cfg.sp3Client.PutObject(r.Context(), item)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error putting object to s3", err)
		return
	}

	video.VideoURL = aws.String(fmt.Sprintf("%s/%s", cfg.s3CfDistribution, fileKeyWithPrefix))

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

}
