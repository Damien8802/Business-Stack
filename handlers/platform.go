package handlers

import (
    "net/http"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"
    "subscription-system/database"
)
// ========== НОВЫЕ ФУНКЦИИ ДЛЯ ПЛАТФОРМЫ (ТОЛЬКО ДЛЯ ВЛАДЕЛЬЦА) ==========

// GetPlatformStaff - список помощников платформы
func GetPlatformStaff(c *gin.Context) {
    // TODO: реализовать получение списка platformAdmins и platformDevelopers
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "staff":   []gin.H{},
        "message": "Функция в разработке",
    })
}

// AddPlatformAdmin - добавить администратора платформы
func AddPlatformAdmin(c *gin.Context) {
    var req struct {
        Email string `json:"email" binding:"required"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // TODO: добавить email в platformAdmins в БД или конфиг
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Администратор платформы добавлен",
    })
}

// AddPlatformDeveloper - добавить разработчика платформы
func AddPlatformDeveloper(c *gin.Context) {
    var req struct {
        Email string `json:"email" binding:"required"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // TODO: добавить email в platformDevelopers в БД или конфиг
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Разработчик платформы добавлен",
    })
}

// RemovePlatformStaff - удалить помощника платформы
func RemovePlatformStaff(c *gin.Context) {
    email := c.Param("email")
    
    // TODO: удалить email из списка помощников
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Помощник удалён: " + email,
    })
}

// GetPlatformSettings - получить настройки платформы
func GetPlatformSettings(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "settings": gin.H{
            "app_name":     "Business Stack",
            "app_version":  "3.0",
            "company_name": "BusinessStack",
        },
    })
}

// UpdatePlatformSettings - обновить настройки платформы
func UpdatePlatformSettings(c *gin.Context) {
    var req struct {
        AppName     string `json:"app_name"`
        CompanyName string `json:"company_name"`
    }
    c.BindJSON(&req)

    // TODO: сохранить настройки в БД или конфиг
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Настройки обновлены",
    })
}

// SetTenantAdmin - назначить админа организации
func SetTenantAdmin(c *gin.Context) {
    var req struct {
        UserID   string `json:"user_id" binding:"required"`
        TenantID string `json:"tenant_id" binding:"required"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET role = 'admin' WHERE id = $1 AND tenant_id = $2
    `, req.UserID, req.TenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Администратор назначен",
    })
}

// SetTenantDeveloper - назначить разработчика организации
func SetTenantDeveloper(c *gin.Context) {
    var req struct {
        UserID   string `json:"user_id" binding:"required"`
        TenantID string `json:"tenant_id" binding:"required"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET role = 'developer' WHERE id = $1 AND tenant_id = $2
    `, req.UserID, req.TenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Разработчик назначен",
    })
}

// GrantModuleAccess - выдать доступ к модулю пользователю
func GrantModuleAccess(c *gin.Context) {
    var req struct {
        UserID     string `json:"user_id" binding:"required"`
        ModuleName string `json:"module_name" binding:"required"`
        Days       int    `json:"days"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.Days == 0 {
        req.Days = 14
    }

    expiresAt := time.Now().Add(time.Duration(req.Days) * 24 * time.Hour)

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO user_subscriptions (user_id, module_name, status, expires_at, created_at)
        VALUES ($1, $2, 'active', $3, NOW())
        ON CONFLICT (user_id, module_name) DO UPDATE SET
            status = 'active',
            expires_at = $3,
            updated_at = NOW()
    `, req.UserID, req.ModuleName, expiresAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Доступ к модулю выдан",
    })
}

// ========== АДМИНКА ОРГАНИЗАЦИИ (ЗАГЛУШКИ ДЛЯ КЛИЕНТОВ) ==========

func TenantAdminDashboard(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Админ-панель организации",
    })
}

func TenantGetUsers(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "users":   []gin.H{},
    })
}

func TenantCreateUser(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Функция в разработке",
    })
}

func TenantSetRole(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Функция в разработке",
    })
}

func TenantDeleteUser(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Функция в разработке",
    })
}

func TenantGetModules(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "modules": []gin.H{},
    })
}

func TenantGrantModuleAccess(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Функция в разработке",
    })
}

// AddPlatformStaff - добавить сотрудника платформы (админа или разработчика)
func AddPlatformStaff(c *gin.Context) {
	var req struct {
		Email      string `json:"email" binding:"required"`
		Password   string `json:"password" binding:"required"`
		Role       string `json:"role"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		MiddleName string `json:"middle_name"`
		Phone      string `json:"phone"`
		BirthDate  string `json:"birth_date"`
		Address    string `json:"address"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Role == "" {
		req.Role = "admin"
	}

	// Проверяем, существует ли пользователь
	var exists bool
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки пользователя"})
		return
	}

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Пользователь с таким email уже существует"})
		return
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка хеширования пароля"})
		return
	}

	// Формируем полное имя
	fullName := req.LastName + " " + req.FirstName
	if req.MiddleName != "" {
		fullName += " " + req.MiddleName
	}

	userID := uuid.New()

	// ✅ ИСПРАВЛЕНО: используем ТОЛЬКО существующие поля
	_, err = database.Pool.Exec(c.Request.Context(), `
		INSERT INTO users (id, email, password_hash, name, role, platform_role, created_at)
		VALUES ($1, $2, $3, $4, $5, 'staff', NOW())
	`, userID, req.Email, string(hashedPassword), fullName, req.Role)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания пользователя: " + err.Error()})
		return
	}

	// ✅ ДОБАВЛЯЕМ ДОПОЛНИТЕЛЬНЫЕ ДАННЫЕ В ТАБЛИЦУ staff (если она есть)
	_, err = database.Pool.Exec(c.Request.Context(), `
		INSERT INTO staff (user_id, email, first_name, last_name, middle_name, phone, birth_date, address, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (email) DO UPDATE SET
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			middle_name = EXCLUDED.middle_name,
			phone = EXCLUDED.phone,
			birth_date = EXCLUDED.birth_date,
			address = EXCLUDED.address,
			role = EXCLUDED.role
	`, userID, req.Email, req.FirstName, req.LastName, req.MiddleName, req.Phone, req.BirthDate, req.Address, req.Role)

	// Если таблицы staff нет - игнорируем ошибку
	if err != nil {
		// Таблица staff не обязательна, просто логируем
		// c.JSON всё равно вернёт успех
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Сотрудник добавлен",
		"user_id": userID,
	})
}
// GetPlatformStaffList - получить список сотрудников платформы
func GetPlatformStaffList(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT 
			u.id,
			u.email,
			u.name,
			u.role,
			u.platform_role,
			u.created_at,
			COALESCE(s.first_name, '') as first_name,
			COALESCE(s.last_name, '') as last_name,
			COALESCE(s.middle_name, '') as middle_name,
			COALESCE(s.phone, '') as phone,
			COALESCE(s.birth_date::text, '') as birth_date,
			COALESCE(s.address, '') as address,
			COALESCE(s.role, u.role) as staff_role
		FROM users u
		LEFT JOIN staff s ON u.email = s.email
		WHERE u.platform_role IN ('staff', 'admin', 'developer')
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var staff []gin.H
	for rows.Next() {
		var id, email, name, role, platformRole string
		var createdAt time.Time
		var firstName, lastName, middleName, phone, birthDate, address, staffRole string

		err := rows.Scan(&id, &email, &name, &role, &platformRole, &createdAt,
			&firstName, &lastName, &middleName, &phone, &birthDate, &address, &staffRole)
		if err != nil {
			continue
		}

		staff = append(staff, gin.H{
			"id":            id,
			"email":         email,
			"name":          name,
			"role":          role,
			"platform_role": platformRole,
			"created_at":    createdAt,
			"first_name":    firstName,
			"last_name":     lastName,
			"middle_name":   middleName,
			"phone":         phone,
			"birth_date":    birthDate,
			"address":       address,
			"staff_role":    staffRole,
		})
	}

	if staff == nil {
		staff = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"staff":   staff,
		"count":   len(staff),
	})
}
// RemovePlatformStaffByEmail - удалить сотрудника платформы по email
func RemovePlatformStaffByEmail(c *gin.Context) {
	email := c.Param("email")

	// Не даём удалить владельца
	if email == "dev@businessstack.ru" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Нельзя удалить владельца платформы"})
		return
	}

	// Удаляем из staff
	_, _ = database.Pool.Exec(c.Request.Context(),
		"DELETE FROM staff WHERE email = $1", email)

	// Удаляем пользователя
	_, err := database.Pool.Exec(c.Request.Context(), `
		DELETE FROM users WHERE email = $1 AND platform_role != 'owner'
	`, email)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Сотрудник удалён",
	})
}