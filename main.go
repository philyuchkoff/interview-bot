package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/telebot.v3"
)

type QuizBot struct {
	questions      []string
	answers        []string
	currentIndex   int
	score          int
	timer          *time.Timer
	timeout        time.Duration
	activeQuizzes  map[int64]*QuizBot // Для хранения состояния по chatID
	userLogsFolder string
}

func main() {
	// Читаем вопросы из файла
	questions, err := readLines("q/questions.txt")
	if err != nil {
		panic(err)
	}

	// Читаем ответы из файла
	answers, err := readLines("q/answ.txt")
	if err != nil {
		panic(err)
	}

	if len(questions) != len(answers) {
		panic("Количество вопросов и ответов не совпадает")
	}

	bot, err := telebot.NewBot(telebot.Settings{
		Token:  "YOUR_TELEGRAM_BOT_TOKEN",
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		panic(err)
	}

	// Создаем папку для логов, если ее нет
	userLogsFolder := "q/user_logs"
	if err := os.MkdirAll(userLogsFolder, 0755); err != nil {
		panic(err)
	}

	activeQuizzes := make(map[int64]*QuizBot)

	bot.Handle("/start", func(c telebot.Context) error {
		chatID := c.Chat().ID

		// Если уже есть активный тест, удаляем его
		if oldQuiz, exists := activeQuizzes[chatID]; exists {
			if oldQuiz.timer != nil {
				oldQuiz.timer.Stop()
			}
			delete(activeQuizzes, chatID)
		}

		// Создаем новый тест
		quiz := &QuizBot{
			questions:      questions,
			answers:        answers,
			currentIndex:   0,
			score:         0,
			timeout:       30 * time.Second, // Таймер 30 секунд на ответ
			activeQuizzes:  activeQuizzes,
			userLogsFolder: userLogsFolder,
		}

		activeQuizzes[chatID] = quiz
		quiz.startTimer(bot, chatID)
		return c.Send("Привет! Начинаем тест. Первый вопрос:\n" + quiz.questions[0] + 
			"\n\nУ вас 30 секунд на ответ. Для отмены теста отправьте /cancel")
	})

	bot.Handle("/cancel", func(c telebot.Context) error {
		chatID := c.Chat().ID
		if quiz, exists := activeQuizzes[chatID]; exists {
			if quiz.timer != nil {
				quiz.timer.Stop()
			}
			delete(activeQuizzes, chatID)
			return c.Send("Тест отменен. Чтобы начать заново, отправьте /start")
		}
		return c.Send("У вас нет активного теста")
	})

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		chatID := c.Chat().ID
		quiz, exists := activeQuizzes[chatID]
		if !exists {
			return nil
		}

		// Останавливаем текущий таймер
		if quiz.timer != nil {
			quiz.timer.Stop()
		}

		userAnswer := strings.TrimSpace(c.Text())
		correctAnswer := strings.TrimSpace(quiz.answers[quiz.currentIndex])

		// Логируем ответ
		quiz.logAnswer(c.Sender().Username, quiz.currentIndex, userAnswer, correctAnswer)

		if strings.EqualFold(userAnswer, correctAnswer) {
			quiz.score++
			_ = c.Send("✅ Правильно!")
		} else {
			_ = c.Send("❌ Не правильно! Правильный ответ: " + correctAnswer)
		}

		quiz.currentIndex++

		// Если вопросы закончились
		if quiz.currentIndex >= len(quiz.questions) {
			delete(activeQuizzes, chatID)
			if quiz.score >= 8 {
				return c.Send(fmt.Sprintf("🎉 Тест пройден! Правильных ответов: %d/%d", quiz.score, len(quiz.questions)))
			} else {
				return c.Send(fmt.Sprintf("😞 Тест не пройден. Правильных ответов: %d/%d", quiz.score, len(quiz.questions)))
			}
		}

		// Задаем следующий вопрос
		quiz.startTimer(bot, chatID)
		return c.Send("Следующий вопрос:\n" + quiz.questions[quiz.currentIndex] + 
			"\n\nУ вас 30 секунд на ответ. Для отмены теста отправьте /cancel")
	})

	bot.Start()
}

func (q *QuizBot) startTimer(bot *telebot.Bot, chatID int64) {
	if q.timer != nil {
		q.timer.Stop()
	}

	q.timer = time.AfterFunc(q.timeout, func() {
		bot.Send(&telebot.Chat{ID: chatID}, "⏰ Время вышло! Переходим к следующему вопросу.")

		q.currentIndex++
		if q.currentIndex >= len(q.questions) {
			bot.Send(&telebot.Chat{ID: chatID}, 
				fmt.Sprintf("Тест завершен. Правильных ответов: %d/%d", q.score, len(q.questions)))
			delete(q.activeQuizzes, chatID)
			return
		}

		bot.Send(&telebot.Chat{ID: chatID}, "Следующий вопрос:\n"+q.questions[q.currentIndex]+
			"\n\nУ вас 30 секунд на ответ. Для отмены теста отправьте /cancel")
		q.startTimer(bot, chatID)
	})
}

func (q *QuizBot) logAnswer(username string, questionIndex int, userAnswer, correctAnswer string) {
	if username == "" {
		username = "unknown"
	}

	logFile := fmt.Sprintf("%s/%s.log", q.userLogsFolder, username)
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logEntry := fmt.Sprintf("[%s] Вопрос %d: Пользователь ответил '%s', правильный ответ '%s'\n",
		timestamp, questionIndex+1, userAnswer, correctAnswer)

	_, _ = file.WriteString(logEntry)
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
