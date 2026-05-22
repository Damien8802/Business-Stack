package handlers

import (
	"context"
	"io"
        "log" 
	"net/http"
	"time"

	"subscription-system/database"
	"subscription-system/models"
	"subscription-system/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UploadAudio - загрузка аудиофайла для распознавания
func UploadAudio(c *gin.Context) {
	accountID := GetAccountID(c)
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id required"})
		return
	}

	// Получаем файл из запроса
	file, err := c.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "файл не найден"})
		return
	}

	// Получаем дополнительные параметры
	customerID := c.PostForm("customer_id")
	dealID := c.PostForm("deal_id")

	// Открываем файл
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ошибка открытия файла"})
		return
	}
	defer src.Close()

	// Читаем файл в память
	audioData, err := io.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ошибка чтения файла"})
		return
	}

	// Создаем запись в БД
	transcriptionID := uuid.New().String()
	_, err = database.Pool.Exec(c.Request.Context(), `
		INSERT INTO audio_transcriptions (id, account_id, customer_id, deal_id, filename, file_size, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'uploaded', NOW(), NOW())
	`, transcriptionID, accountID, customerID, dealID, file.Filename, file.Size)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Запускаем обработку в фоне
	go processAudio(transcriptionID, audioData, file.Filename)

	c.JSON(http.StatusOK, gin.H{
		"message":          "файл загружен, обработка начата",
		"transcription_id": transcriptionID,
	})
}

// processAudio - фоновая обработка аудио
func processAudio(transcriptionID string, audioData []byte, filename string) {
	ctx := context.Background()

	// Получаем SpeechKit сервис
	speechKit := &services.SpeechKitService{
		// TODO: добавить инициализацию с cfg
	}

	// Обновляем статус
	database.Pool.Exec(ctx, "UPDATE audio_transcriptions SET status = 'processing' WHERE id = $1", transcriptionID)

	// Распознаем речь
	text, err := speechKit.TranscribeAudio(ctx, audioData, filename)
	if err != nil {
		database.Pool.Exec(ctx, "UPDATE audio_transcriptions SET status = 'failed' WHERE id = $1", transcriptionID)
		return
	}

	// Анализируем тональность
	sentiment, _ := speechKit.AnalyzeSentiment(ctx, text)

	// Создаем краткое содержание
	summary, _ := speechKit.GenerateSummary(ctx, text)

	// Обновляем запись
	database.Pool.Exec(ctx, `
		UPDATE audio_transcriptions 
		SET transcription = $1, sentiment = $2, summary = $3, status = 'completed', updated_at = NOW()
		WHERE id = $4
	`, text, sentiment, summary, transcriptionID)
}

// GetTranscriptions - список транскрипций
func GetTranscriptions(c *gin.Context) {
    accountID := GetAccountID(c)
    if accountID == "" {
        // В режиме SkipAuth используем тестовый account_id
        accountID = "00000000-0000-0000-0000-000000000001"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, customer_id, deal_id, filename, file_size, 
               COALESCE(duration, 0) as duration,
               COALESCE(transcription, '') as transcription,
               COALESCE(summary, '') as summary,
               COALESCE(sentiment, '') as sentiment,
               status, created_at
        FROM audio_transcriptions
        WHERE account_id = $1
        ORDER BY created_at DESC
    `, accountID)

    if err != nil {
        log.Printf("❌ Ошибка получения транскрипций: %v", err)
        // Возвращаем пустой массив, а не ошибку
        c.JSON(http.StatusOK, gin.H{"transcriptions": []gin.H{}})
        return
    }
    defer rows.Close()

    var transcriptions []gin.H
    for rows.Next() {
        var id, customerID, dealID, filename, transcription, summary, sentiment, status string
        var fileSize, duration int
        var createdAt time.Time

        err := rows.Scan(&id, &customerID, &dealID, &filename, &fileSize, &duration,
            &transcription, &summary, &sentiment, &status, &createdAt)
        if err != nil {
            log.Printf("❌ Ошибка сканирования: %v", err)
            continue
        }

        transcriptions = append(transcriptions, gin.H{
            "id":            id,
            "customer_id":   customerID,
            "deal_id":       dealID,
            "filename":      filename,
            "file_size":     fileSize,
            "duration":      duration,
            "transcription": transcription,
            "summary":       summary,
            "sentiment":     sentiment,
            "status":        status,
            "created_at":    createdAt,
        })
    }

    c.JSON(http.StatusOK, gin.H{"transcriptions": transcriptions})
}

// GetTranscriptionByID - получение конкретной транскрипции
func GetTranscriptionByID(c *gin.Context) {
	accountID := GetAccountID(c)
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id required"})
		return
	}

	id := c.Param("id")

	var transcription models.AudioTranscription
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT id, account_id, customer_id, deal_id, filename, file_size, duration,
		       audio_url, transcription, summary, sentiment, key_points, action_items, status, created_at
		FROM audio_transcriptions
		WHERE id = $1 AND account_id = $2
	`, id, accountID).Scan(
		&transcription.ID, &transcription.AccountID, &transcription.CustomerID, &transcription.DealID,
		&transcription.Filename, &transcription.FileSize, &transcription.Duration,
		&transcription.AudioURL, &transcription.Transcription, &transcription.Summary,
		&transcription.Sentiment, &transcription.KeyPoints, &transcription.ActionItems,
		&transcription.Status, &transcription.CreatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "транскрипция не найдена"})
		return
	}

	c.JSON(http.StatusOK, transcription)
}

// TranscriptionsPage - отображение страницы с транскрипциями
func TranscriptionsPage(c *gin.Context) {
    userEmail := c.GetString("userEmail")
    if userEmail == "" {
        userEmail = "admin@example.com"
    }

    userName := c.GetString("userName")
    if userName == "" {
        userName = "Администратор"
    }

    c.HTML(http.StatusOK, "transcriptions.html", gin.H{
        "Title":     "Транскрибация звонков - Business Stack",
        "Version":   "3.0",
        "UserEmail": userEmail,
        "UserName":  userName,
        "IsAdmin":   true,
    })
}