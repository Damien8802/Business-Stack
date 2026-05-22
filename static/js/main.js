// Основной JavaScript файл Business Stack

class Business StackApp {
    constructor() {
        this.init();
    }

    init() {
        console.log('Business Stack App инициализирован');
        
        // Инициализация компонентов
        this.initNotifications();
        this.initForms();
        this.initCharts();
        this.initSidebar();
        this.initTheme();
        
        // Обновление времени
        this.updateDateTime();
        setInterval(() => this.updateDateTime(), 1000);
    }

    initNotifications() {
        // Показ уведомлений
        window.showNotification = (message, type = 'info', duration = 5000) => {
            const toastId = 'toast-' + Date.now();
            const toastHtml = `
                <div id="${toastId}" class="toast align-items-center text-bg-${type} border-0" role="alert">
                    <div class="d-flex">
                        <div class="toast-body">
                            ${message}
                        </div>
                        <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button>
                    </div>
                </div>
            `;
            
            const container = document.querySelector('.toast-container') || this.createToastContainer();
            container.insertAdjacentHTML('beforeend', toastHtml);
            
            const toastEl = document.getElementById(toastId);
            const toast = new bootstrap.Toast(toastEl, { delay: duration });
            toast.show();
            
            toastEl.addEventListener('hidden.bs.toast', () => {
                toastEl.remove();
            });
        };
    }

    createToastContainer() {
        const container = document.createElement('div');
        container.className = 'toast-container position-fixed top-0 end-0 p-3';
        document.body.appendChild(container);
        return container;
    }

    initForms() {
        // Обработка форм с индикацией загрузки
        document.querySelectorAll('form').forEach(form => {
            form.addEventListener('submit', (e) => {
                const submitBtn = form.querySelector('button[type="submit"]');
                if (submitBtn) {
                    const originalText = submitBtn.innerHTML;
                    submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Обработка...';
                    submitBtn.disabled = true;
                    
                    // Восстановление кнопки через 10 секунд (на случай ошибки)
                    setTimeout(() => {
                        submitBtn.innerHTML = originalText;
                        submitBtn.disabled = false;
                    }, 10000);
                }
            });
        });
        
        // Валидация форм
        document.querySelectorAll('.needs-validation').forEach(form => {
            form.addEventListener('submit', (e) => {
                if (!form.checkValidity()) {
                    e.preventDefault();
                    e.stopPropagation();
                }
                form.classList.add('was-validated');
            });
        });
    }

    initCharts() {
        // Динамическая загрузка Chart.js если нужен
        if (typeof Chart === 'undefined' && document.querySelector('canvas')) {
            const script = document.createElement('script');
            script.src = 'https://cdn.jsdelivr.net/npm/chart.js';
            script.onload = () => console.log('Chart.js loaded');
            document.head.appendChild(script);
        }
    }

    initSidebar() {
        // Переключение сайдбара на мобильных
        const sidebarToggle = document.querySelector('[data-bs-toggle="sidebar"]');
        if (sidebarToggle) {
            sidebarToggle.addEventListener('click', () => {
                const sidebar = document.querySelector('.sidebar');
                sidebar.classList.toggle('active');
            });
        }
        
        // Активный пункт меню
        const currentPath = window.location.pathname;
        document.querySelectorAll('.sidebar .nav-link').forEach(link => {
            if (link.getAttribute('href') === currentPath) {
                link.classList.add('active');
            }
        });
    }

    initTheme() {
        // Переключение темы
        const themeToggle = document.querySelector('[data-bs-theme-toggle]');
        if (themeToggle) {
            themeToggle.addEventListener('click', () => {
                const currentTheme = document.documentElement.getAttribute('data-bs-theme');
                const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
                document.documentElement.setAttribute('data-bs-theme', newTheme);
                localStorage.setItem('theme', newTheme);
                showNotification(`Тема изменена на ${newTheme === 'dark' ? 'темную' : 'светлую'}`, 'info');
            });
        }
        
        // Загрузка сохраненной темы
        const savedTheme = localStorage.getItem('theme');
        if (savedTheme) {
            document.documentElement.setAttribute('data-bs-theme', savedTheme);
        }
    }

    updateDateTime() {
        const now = new Date();
        const options = {
            weekday: 'long',
            year: 'numeric',
            month: 'long',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            timeZone: 'Europe/Moscow'
        };
        
        document.querySelectorAll('.current-datetime').forEach(el => {
            el.textContent = now.toLocaleDateString('ru-RU', options);
        });
        
        document.querySelectorAll('.current-time').forEach(el => {
            el.textContent = now.toLocaleTimeString('ru-RU');
        });
    }

    // API методы
    async fetchData(url, options = {}) {
        try {
            const response = await fetch(url, {
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers
                },
                ...options
            });
            
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            
            return await response.json();
        } catch (error) {
            console.error('Fetch error:', error);
            showNotification('Ошибка загрузки данных', 'danger');
            throw error;
        }
    }

    // Экспорт данных
    exportToCSV(data, filename) {
        const csvContent = "data:text/csv;charset=utf-8," 
            + data.map(row => row.join(",")).join("\n");
        
        const encodedUri = encodeURI(csvContent);
        const link = document.createElement("a");
        link.setAttribute("href", encodedUri);
        link.setAttribute("download", filename);
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }
}

// Инициализация приложения после загрузки DOM
document.addEventListener('DOMContentLoaded', () => {
    window.app = new Business StackApp();
    
    // Анимация элементов при прокрутке
    const observerOptions = {
        threshold: 0.1,
        rootMargin: '0px 0px -50px 0px'
    };
    
    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('animate__animated', 'animate__fadeInUp');
                observer.unobserve(entry.target);
            }
        });
    }, observerOptions);
    
    document.querySelectorAll('.animate-on-scroll').forEach(el => {
        observer.observe(el);
    });
});

// Глобальные утилиты
window.formatCurrency = (amount, currency = 'RUB') => {
    return new Intl.NumberFormat('ru-RU', {
        style: 'currency',
        currency: currency
    }).format(amount);
};

window.formatDate = (dateString) => {
    const date = new Date(dateString);
    return date.toLocaleDateString('ru-RU', {
        year: 'numeric',
        month: 'long',
        day: 'numeric'
    });
};

// Обработка ошибок
window.addEventListener('error', (event) => {
    console.error('Global error:', event.error);
    showNotification('Произошла ошибка. Пожалуйста, обновите страницу.', 'danger');
});
