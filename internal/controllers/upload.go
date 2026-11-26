package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

const maxImageBytes = 2 << 20

func saveValidatedImage(c *gin.Context, prefix string) (string, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageBytes+256*1024)
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		return "", err
	}
	defer file.Close()

	if header.Size > maxImageBytes {
		return "", errors.New("image too large")
	}
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	detected := http.DetectContentType(buf[:n])
	if detected != "image/jpeg" && detected != "image/png" && detected != "image/gif" {
		return "", errors.New("unsupported image type")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	cfg, format, err := image.DecodeConfig(file)
	if err != nil || (format != "jpeg" && format != "png" && format != "gif") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 6000 || cfg.Height > 6000 {
		return "", errors.New("invalid image")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	ext := map[string]string{"jpeg": ".jpg", "png": ".png", "gif": ".gif"}[format]
	filename := prefix + hex.EncodeToString(random[:]) + ext
	dir := "./public/uploads"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	out, err := os.OpenFile(filepath.Join(dir, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, io.LimitReader(file, maxImageBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(filepath.Join(dir, filename))
		if copyErr != nil { return "", copyErr }
		return "", closeErr
	}
	return "/uploads/" + filename, nil
}
