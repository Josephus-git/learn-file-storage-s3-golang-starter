package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FFProbeResult represents the overall structure of the ffprobe JSON output.
type FFProbeResult struct {
	Streams []Stream `json:"streams"`
}

// Stream represents a single stream within the ffprobe output (e.g., video, audio).
type Stream struct {
	CodecType string `json:"codec_type"` // "video", "audio", etc.
	Width     int    `json:"width"`      // Width of the video stream
	Height    int    `json:"height"`     // Height of the video stream
}

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil {
		panic("failed to generate random bytes")
	}
	id := base64.RawURLEncoding.EncodeToString(base)

	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", id, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func getAssetPathWithPrefix(prefix, assetPath string) string {
	return fmt.Sprintf("/%s/%s", prefix, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run ffprobe command: %w", err)
	}

	// unmarshal the JSON output from the buffer into our Go stuct
	var result FFProbeResult
	err = json.Unmarshal(out.Bytes(), &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse ffprobe JSON output: %w", err)
	}

	// Iterate through the streams to find a video stream.
	var videoWidth, videoHeight int
	foundVideoStream := false
	for _, stream := range result.Streams {
		if stream.CodecType == "video" {
			videoWidth = stream.Width
			videoHeight = stream.Height
			foundVideoStream = true
			break // Found the first video stream, we'll use that.
		}
	}

	// If no video stream was found, return an error.
	if !foundVideoStream {
		return "", fmt.Errorf("no video stream found in %s", filePath)
	}

	// Handle cases where width or height might be zero to avoid division by zero.
	if videoWidth == 0 || videoHeight == 0 {
		return "other", fmt.Errorf("video dimensions are zero (width: %d, height: %d) for %s", videoWidth, videoHeight, filePath)
	}

	// Calculate the aspect ratio.
	ratio := float64(videoWidth) / float64(videoHeight)

	// Define a small tolerance for floating-point comparisons.
	const tolerance = 0.01

	// Determine the aspect ratio string.
	// 16:9 ratio is approximately 1.777...
	if math.Abs(ratio-(16.0/9.0)) < tolerance {
		return "16:9", nil
	}
	// 9:16 ratio is approximately 0.5625
	if math.Abs(ratio-(9.0/16.0)) < tolerance {
		return "9:16", nil
	}

	// If it's neither 16:9 nor 9:16, return "other".
	return "other", nil
}

func processVidoeForFastStart(filePath string) (string, error) {
	newFilePath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", newFilePath)

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run ffmpeg command: %w", err)
	}

	return newFilePath, nil
}

/*
func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignedClient := s3.NewPresignClient(s3Client)

	resp, err := presignedClient.PresignGetObject(context.TODO(),
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}

	// Return the generated URL
	return resp.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return database.Video{}, fmt.Errorf("video URL is nil for video '%s'", video.ID)
	}
	parts := strings.SplitN(*video.VideoURL, ",", 2)
	if len(parts) != 2 {
		return database.Video{}, fmt.Errorf("invalid video URL format: expected 'bucket,key', got '%s'", *video.VideoURL)
	}

	bucket := strings.TrimSpace(parts[0])
	key := strings.TrimSpace(parts[1])

	// generate presigned url
	const presignExpiration = 15 * time.Minute
	presignedURL, err := generatePresignedURL(cfg.sp3Client, bucket, key, presignExpiration)
	if err != nil {
		return database.Video{}, fmt.Errorf("failed to generate presigned URL for video '%s': %w", video.ID, err)
	}

	// Set the VideoURL field of the video to the presigned URL
	video.VideoURL = &presignedURL

	return video, nil
}
*/
