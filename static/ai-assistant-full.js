// ============================================
// МАКСИМАЛЬНЫЙ AI ASSISTANT Business Stack v1.0
// ============================================

(function() {
    // Конфигурация
    const config = {
        apiUrl: '/api/ai/assistant',
        widgetId: 'aiAssistantFull',
        isOpen: false,
        messages: [],
        conversationId: localStorage.getItem('ai_conversation_id') || 'session_' + Date.now()
    };
    
    // Сохраняем ID сессии
    localStorage.setItem('ai_conversation_id', config.conversationId);
    
    // HTML виджета
    const widgetHTML = `
    <style>
        /* Плавающая кнопка */
        .ai-fab-full {
            position: fixed;
            bottom: 30px;
            right: 30px;
            width: 65px;
            height: 65px;
            border-radius: 50%;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            cursor: pointer;
            z-index: 99999;
            box-shadow: 0 4px 20px rgba(0,0,0,0.3);
            font-size: 28px;
            transition: all 0.3s cubic-bezier(0.68, -0.55, 0.265, 1.55);
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .ai-fab-full:hover {
            transform: scale(1.15) rotate(5deg);
            box-shadow: 0 8px 30px rgba(102,126,234,0.5);
        }
        
        /* Главный виджет */
        .ai-widget-full {
            position: fixed;
            bottom: 110px;
            right: 30px;
            width: 420px;
            height: 600px;
            background: white;
            border-radius: 24px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            display: none;
            flex-direction: column;
            z-index: 100000;
            overflow: hidden;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            animation: slideInRight 0.3s ease;
        }
        .ai-widget-full.open {
            display: flex;
        }
        @keyframes slideInRight {
            from {
                opacity: 0;
                transform: translateX(50px);
            }
            to {
                opacity: 1;
                transform: translateX(0);
            }
        }
        
        /* Шапка виджета */
        .ai-widget-header-full {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 18px 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            cursor: move;
        }
        .ai-widget-header-full h4 {
            margin: 0;
            font-size: 18px;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .ai-widget-header-full .ai-status {
            font-size: 10px;
            background: rgba(255,255,255,0.3);
            padding: 3px 8px;
            border-radius: 20px;
        }
        .ai-widget-header-full button {
            background: none;
            border: none;
            color: white;
            font-size: 24px;
            cursor: pointer;
            opacity: 0.8;
            transition: opacity 0.2s;
        }
        .ai-widget-header-full button:hover {
            opacity: 1;
        }
        
        /* Область сообщений */
        .ai-widget-messages-full {
            flex: 1;
            overflow-y: auto;
            padding: 20px;
            background: #f8f9fa;
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        
        /* Сообщения */
        .ai-message-full {
            max-width: 85%;
            padding: 12px 16px;
            border-radius: 18px;
            word-wrap: break-word;
            animation: fadeIn 0.3s ease;
            font-size: 14px;
            line-height: 1.5;
        }
        .ai-message-full.user {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            align-self: flex-end;
            border-bottom-right-radius: 4px;
        }
        .ai-message-full.assistant {
            background: white;
            color: #212529;
            align-self: flex-start;
            border-bottom-left-radius: 4px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.05);
        }
        .ai-message-full.system {
            background: #fff3cd;
            color: #856404;
            text-align: center;
            font-size: 12px;
            align-self: center;
            max-width: 90%;
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        
        /* Индикатор печати */
        .ai-typing-full {
            display: flex;
            gap: 6px;
            padding: 12px 16px;
            background: white;
            border-radius: 18px;
            width: fit-content;
            align-self: flex-start;
            box-shadow: 0 2px 5px rgba(0,0,0,0.05);
        }
        .ai-typing-full span {
            width: 8px;
            height: 8px;
            background: #667eea;
            border-radius: 50%;
            animation: typingBounce 1.4s infinite;
        }
        .ai-typing-full span:nth-child(2) { animation-delay: 0.2s; }
        .ai-typing-full span:nth-child(3) { animation-delay: 0.4s; }
        @keyframes typingBounce {
            0%, 60%, 100% { transform: translateY(0); }
            30% { transform: translateY(-12px); }
        }
        
        /* Быстрые действия */
        .ai-quick-actions-full {
            padding: 12px 15px;
            background: white;
            border-top: 1px solid #e9ecef;
            display: flex;
            gap: 8px;
            flex-wrap: wrap;
        }
        .ai-quick-action-full {
            background: #f0f0f0;
            padding: 6px 14px;
            border-radius: 20px;
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 6px;
        }
        .ai-quick-action-full:hover {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            transform: translateY(-2px);
        }
        
        /* Поле ввода */
        .ai-widget-input-full {
            padding: 15px;
            background: white;
            border-top: 1px solid #dee2e6;
            display: flex;
            gap: 10px;
            align-items: center;
        }
        .ai-widget-input-full input {
            flex: 1;
            padding: 12px 16px;
            border: 1px solid #e0e0e0;
            border-radius: 25px;
            outline: none;
            font-size: 14px;
            transition: all 0.2s;
        }
        .ai-widget-input-full input:focus {
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102,126,234,0.1);
        }
        .ai-widget-input-full button {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 12px 20px;
            border-radius: 25px;
            cursor: pointer;
            transition: all 0.2s;
            font-weight: 500;
        }
        .ai-widget-input-full button:hover {
            transform: scale(1.02);
            box-shadow: 0 2px 8px rgba(102,126,234,0.4);
        }
        
        /* Голосовой ввод */
        .ai-voice-btn-full {
            background: #f0f0f0 !important;
            color: #667eea !important;
        }
        .ai-voice-btn-full.recording {
            background: #dc3545 !important;
            color: white !important;
            animation: pulse 1s infinite;
        }
        @keyframes pulse {
            0% { transform: scale(1); }
            50% { transform: scale(1.05); }
            100% { transform: scale(1); }
        }
        
        /* Мобильная адаптация */
        @media (max-width: 768px) {
            .ai-widget-full {
                width: calc(100% - 20px);
                right: 10px;
                bottom: 80px;
                height: 500px;
            }
        }
        
        /* Скроллбар */
        .ai-widget-messages-full::-webkit-scrollbar {
            width: 6px;
        }
        .ai-widget-messages-full::-webkit-scrollbar-track {
            background: #f1f1f1;
            border-radius: 3px;
        }
        .ai-widget-messages-full::-webkit-scrollbar-thumb {
            background: #667eea;
            border-radius: 3px;
        }
    </style>
    
    <button class="ai-fab-full" id="aiFabFull">
        🤖
    </button>
    
    <div class="ai-widget-full" id="aiWidgetFull">
        <div class="ai-widget-header-full" id="aiWidgetHeaderFull">
            <h4>
                <span>🤖</span>
                AI Assistant Business Stack
                <span class="ai-status">⚡ online</span>
            </h4>
            <div>
                <button id="aiClearChatFull" title="Очистить чат">🗑️</button>
                <button id="aiMinimizeFull" title="Свернуть">−</button>
                <button id="aiCloseFull" title="Закрыть">×</button>
            </div>
        </div>
        
        <div class="ai-widget-messages-full" id="aiMessagesFull">
            <div class="ai-message-full assistant">
                <strong>👋 Привет! Я AI-ассистент Business Stack</strong><br><br>
                Я помогаю управлять CRM, создавать сделки, искать клиентов и многое другое.<br><br>
                <strong>📋 Что я умею:</strong><br>
                • 📝 <strong>Создавать сделки</strong> — напишите "создай сделку для ООО Ромашка на 1.5 млн"<br>
                • 🔍 <strong>Искать клиентов</strong> — напишите "найди клиента Иванов"<br>
                • 📊 <strong>Показывать сделки</strong> — напишите "покажи мои сделки"<br>
                • ✏️ <strong>Изменять статус</strong> — напишите "измени статус сделки Ромашка на closed_won"<br>
                • ✅ <strong>Создавать задачи</strong> — напишите "создай задачу для Иванова"<br><br>
                💡 <strong>Совет:</strong> Нажмите на микрофон для голосового ввода!<br><br>
                Чем могу помочь?
            </div>
        </div>
        
        <div class="ai-quick-actions-full">
            <span class="ai-quick-action-full" data-action="create_deal">📝 Создать сделку</span>
            <span class="ai-quick-action-full" data-action="find_customer">🔍 Найти клиента</span>
            <span class="ai-quick-action-full" data-action="show_deals">📊 Мои сделки</span>
            <span class="ai-quick-action-full" data-action="show_stats">📈 Статистика</span>
            <span class="ai-quick-action-full" data-action="help">❓ Помощь</span>
        </div>
        
        <div class="ai-widget-input-full">
            <input type="text" id="aiInputFull" placeholder="Напишите сообщение..." autocomplete="off">
            <button id="aiVoiceBtnFull" class="ai-voice-btn-full" title="Голосовой ввод">🎤</button>
            <button id="aiSendBtnFull">📤 Отправить</button>
        </div>
    </div>
    `;
    
    // Добавляем виджет на страницу
    const container = document.createElement('div');
    container.id = config.widgetId;
    container.innerHTML = widgetHTML;
    document.body.appendChild(container);
    
    // DOM элементы
    const fab = document.getElementById('aiFabFull');
    const widget = document.getElementById('aiWidgetFull');
    const closeBtn = document.getElementById('aiCloseFull');
    const minimizeBtn = document.getElementById('aiMinimizeFull');
    const clearBtn = document.getElementById('aiClearChatFull');
    const input = document.getElementById('aiInputFull');
    const sendBtn = document.getElementById('aiSendBtnFull');
    const voiceBtn = document.getElementById('aiVoiceBtnFull');
    const messagesContainer = document.getElementById('aiMessagesFull');
    const header = document.getElementById('aiWidgetHeaderFull');
    
    // Переменные для перетаскивания
    let isDragging = false;
    let dragOffsetX, dragOffsetY;
    
    if (header) {
        header.addEventListener('mousedown', startDrag);
        document.addEventListener('mousemove', onDrag);
        document.addEventListener('mouseup', stopDrag);
    }
    
    function startDrag(e) {
        if (e.target.closest('button')) return;
        isDragging = true;
        dragOffsetX = e.clientX - widget.offsetLeft;
        dragOffsetY = e.clientY - widget.offsetTop;
        widget.style.position = 'fixed';
        widget.style.cursor = 'grabbing';
    }
    
    function onDrag(e) {
        if (!isDragging) return;
        let left = e.clientX - dragOffsetX;
        let top = e.clientY - dragOffsetY;
        left = Math.max(0, Math.min(left, window.innerWidth - widget.offsetWidth));
        top = Math.max(0, Math.min(top, window.innerHeight - widget.offsetHeight));
        widget.style.left = left + 'px';
        widget.style.top = top + 'px';
        widget.style.right = 'auto';
        widget.style.bottom = 'auto';
    }
    
    function stopDrag() {
        isDragging = false;
        if (widget) widget.style.cursor = '';
    }
    
    // Функции виджета
    function openWidget() {
        widget.classList.add('open');
        config.isOpen = true;
        input.focus();
    }
    
    function closeWidget() {
        widget.classList.remove('open');
        config.isOpen = false;
    }
    
    function minimizeWidget() {
        widget.classList.remove('open');
        config.isOpen = false;
    }
    
    function clearChat() {
        const messages = messagesContainer.querySelectorAll('.ai-message-full:not(:first-child)');
        messages.forEach(msg => msg.remove());
        addMessage('🧹 Чат очищен. Чем могу помочь?', 'system');
    }
    
    function addMessage(text, sender) {
        const msgDiv = document.createElement('div');
        msgDiv.className = `ai-message-full ${sender}`;
        msgDiv.innerHTML = text.replace(/\n/g, '<br>');
        messagesContainer.appendChild(msgDiv);
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
        config.messages.push({ text, sender, timestamp: Date.now() });
    }
    
    function showTyping() {
        const typingDiv = document.createElement('div');
        typingDiv.className = 'ai-typing-full';
        typingDiv.id = 'aiTypingFull';
        typingDiv.innerHTML = '<span></span><span></span><span></span>';
        messagesContainer.appendChild(typingDiv);
        messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }
    
    function hideTyping() {
        const typing = document.getElementById('aiTypingFull');
        if (typing) typing.remove();
    }
    
    async function sendMessage() {
        const message = input.value.trim();
        if (!message) return;
        
        addMessage(message, 'user');
        input.value = '';
        showTyping();
        
        try {
            const token = localStorage.getItem('token');
            const response = await fetch(config.apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + (token || '')
                },
                body: JSON.stringify({
                    message: message,
                    conversation_id: config.conversationId
                })
            });
            
            const data = await response.json();
            hideTyping();
            addMessage(data.response, 'assistant');
        } catch (error) {
            hideTyping();
            addMessage('❌ Ошибка соединения. Попробуйте позже.', 'system');
        }
    }
    
    // Голосовой ввод
    let recognition = null;
    let isRecording = false;
    
    function initVoiceRecognition() {
        if (!('webkitSpeechRecognition' in window) && !('SpeechRecognition' in window)) {
            voiceBtn.style.display = 'none';
            return;
        }
        
        const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
        recognition = new SpeechRecognition();
        recognition.lang = 'ru-RU';
        recognition.continuous = false;
        recognition.interimResults = false;
        
        recognition.onstart = () => {
            isRecording = true;
            voiceBtn.classList.add('recording');
            voiceBtn.innerHTML = '🔴';
        };
        
        recognition.onend = () => {
            isRecording = false;
            voiceBtn.classList.remove('recording');
            voiceBtn.innerHTML = '🎤';
        };
        
        recognition.onresult = (event) => {
            const transcript = event.results[0][0].transcript;
            input.value = transcript;
            sendMessage();
        };
        
        recognition.onerror = (event) => {
            console.error('Voice error:', event.error);
            isRecording = false;
            voiceBtn.classList.remove('recording');
            voiceBtn.innerHTML = '🎤';
        };
    }
    
    function toggleVoiceInput() {
        if (!recognition) {
            initVoiceRecognition();
        }
        if (isRecording) {
            recognition.stop();
        } else {
            try {
                recognition.start();
            } catch (e) {
                console.error('Start failed:', e);
            }
        }
    }
    
    // Быстрые действия
    const quickActions = {
        create_deal: 'создай новую сделку',
        find_customer: 'найди клиента',
        show_deals: 'покажи мои сделки',
        show_stats: 'покажи статистику',
        help: 'помощь'
    };
    
    document.querySelectorAll('.ai-quick-action-full').forEach(btn => {
        btn.addEventListener('click', () => {
            const action = btn.dataset.action;
            if (quickActions[action]) {
                input.value = quickActions[action];
                sendMessage();
            }
        });
    });
    
    // Обработчики событий
    fab.addEventListener('click', openWidget);
    closeBtn.addEventListener('click', closeWidget);
    minimizeBtn.addEventListener('click', minimizeWidget);
    clearBtn.addEventListener('click', clearChat);
    sendBtn.addEventListener('click', sendMessage);
    voiceBtn.addEventListener('click', toggleVoiceInput);
    input.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') sendMessage();
    });
    
    // Горячие клавиши
    document.addEventListener('keydown', (e) => {
        if (e.ctrlKey && e.key === 'a') {
            e.preventDefault();
            if (config.isOpen) closeWidget();
            else openWidget();
        }
        if (e.key === 'Escape' && config.isOpen) closeWidget();
    });
    
    // Инициализация
    initVoiceRecognition();
    console.log('🚀 AI Assistant Business Stack v1.0 загружен!');
})();