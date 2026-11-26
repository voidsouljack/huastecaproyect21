package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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

const maxImageSize int64 = 2 << 20 // 2 MiB

func saveValidatedImage(c *gin.Context) (string, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageSize+512*1024)

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		return "", fmt.Errorf("archivo de imagen requerido")
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxImageSize {
		return "", fmt.Errorf("la imagen debe pesar entre 1 byte y 2 MB")
	}

	sniff := make([]byte, 512)
	n, err := file.Read(sniff)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("no se pudo leer la imagen")
	}
	contentType := http.DetectContentType(sniff[:n])

	extByType := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
	}
	ext, ok := extByType[contentType]
	if !ok {
		return "", fmt.Errorf("solo se permiten JPEG, PNG y GIF")
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("no se pudo leer el archivo")
	}
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return "", fmt.Errorf("archivo de imagen inválido")
	}
	expectedFormat := map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/gif": "gif"}[contentType]
	if format != expectedFormat {
		return "", fmt.Errorf("el contenido no coincide con su tipo")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 12_000_000 {
		return "", fmt.Errorf("dimensiones de imagen no permitidas")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("no se pudo leer la imagen")
	}
	if _, decodedFormat, err := image.Decode(file); err != nil || decodedFormat != expectedFormat {
		return "", fmt.Errorf("imagen corrupta o formato inválido")
	}

	randomName := make([]byte, 16)
	if _, err := rand.Read(randomName); err != nil {
		return "", fmt.Errorf("no se pudo generar el nombre de archivo")
	}
	filename := hex.EncodeToString(randomName) + ext

	if err := os.MkdirAll("./public/uploads", 0755); err != nil {
		return "", fmt.Errorf("no se pudo crear directorio de uploads")
	}
	destination := filepath.Join("./public/uploads", filename)
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", fmt.Errorf("no se pudo guardar la imagen")
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		out.Close()
		os.Remove(destination)
		return "", fmt.Errorf("no se pudo leer la imagen")
	}
	written, copyErr := io.Copy(out, io.LimitReader(file, maxImageSize+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > maxImageSize {
		os.Remove(destination)
		return "", fmt.Errorf("no se pudo guardar una imagen válida")
	}

	return filename, nil
}
