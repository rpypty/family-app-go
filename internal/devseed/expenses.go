package devseed

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

type ExpensesService interface {
	CreateCategory(ctx context.Context, input expensesdomain.CreateCategoryInput) (*expensesdomain.Category, error)
	CreateExpense(ctx context.Context, input expensesdomain.CreateExpenseInput) (*expensesdomain.ExpenseWithCategories, error)
}

type Config struct {
	Enabled          bool
	LookbackMonths   int
	MinCategories    int
	MaxCategories    int
	MaxDailyExpenses int
	Currency         string
}

type SeedFamilyInput struct {
	FamilyID string
	UserID   string
	Currency string
}

type SeedFamilyResult struct {
	CategoriesCreated int
	ExpensesCreated   int
	From              time.Time
	To                time.Time
}

type ExpenseSeeder struct {
	expenses ExpensesService
	cfg      Config
	now      func() time.Time
	rng      *rand.Rand
	mu       sync.Mutex
}

func NewExpenseSeeder(expenses ExpensesService, cfg Config) *ExpenseSeeder {
	return NewExpenseSeederWithClockAndRand(expenses, cfg, time.Now, rand.New(rand.NewSource(time.Now().UnixNano())))
}

func NewExpenseSeederWithClockAndRand(expenses ExpensesService, cfg Config, now func() time.Time, rng *rand.Rand) *ExpenseSeeder {
	if now == nil {
		now = time.Now
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &ExpenseSeeder{
		expenses: expenses,
		cfg:      normalizeConfig(cfg),
		now:      now,
		rng:      rng,
	}
}

func (s *ExpenseSeeder) SeedFamily(ctx context.Context, input SeedFamilyInput) (SeedFamilyResult, error) {
	if s == nil || !s.cfg.Enabled || s.expenses == nil {
		return SeedFamilyResult{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	today := dateOnlyUTC(s.now())
	from := today.AddDate(0, -s.cfg.LookbackMonths, 0)
	categoryDefs := s.pickCategories()

	createdCategories := make([]expensesdomain.Category, 0, len(categoryDefs))
	for _, category := range categoryDefs {
		created, err := s.expenses.CreateCategory(ctx, expensesdomain.CreateCategoryInput{
			FamilyID: input.FamilyID,
			Name:     category.name,
			Color:    stringPtr(category.color),
		})
		if err != nil {
			return SeedFamilyResult{}, err
		}
		createdCategories = append(createdCategories, *created)
	}

	currency := input.Currency
	if currency == "" {
		currency = s.cfg.Currency
	}

	expensesCreated := 0
	for day := from; !day.After(today); day = day.AddDate(0, 0, 1) {
		expensesInDay := s.rng.Intn(s.cfg.MaxDailyExpenses + 1)
		for i := 0; i < expensesInDay; i++ {
			categoryIndex := s.rng.Intn(len(createdCategories))
			category := createdCategories[categoryIndex]
			definition := categoryDefs[categoryIndex]
			title := definition.titles[s.rng.Intn(len(definition.titles))]
			amount := randomAmount(s.rng, definition.minAmount, definition.maxAmount)

			if _, err := s.expenses.CreateExpense(ctx, expensesdomain.CreateExpenseInput{
				FamilyID:     input.FamilyID,
				UserID:       input.UserID,
				Date:         day,
				Amount:       amount,
				Currency:     currency,
				BaseCurrency: currency,
				Title:        title,
				CategoryIDs:  []string{category.ID},
			}); err != nil {
				return SeedFamilyResult{}, err
			}
			expensesCreated++
		}
	}

	return SeedFamilyResult{
		CategoriesCreated: len(createdCategories),
		ExpensesCreated:   expensesCreated,
		From:              from,
		To:                today,
	}, nil
}

func (s *ExpenseSeeder) pickCategories() []mockCategory {
	count := s.cfg.MinCategories
	if s.cfg.MaxCategories > s.cfg.MinCategories {
		count += s.rng.Intn(s.cfg.MaxCategories - s.cfg.MinCategories + 1)
	}

	indexes := s.rng.Perm(len(mockCategories))
	result := make([]mockCategory, 0, count)
	for _, index := range indexes[:count] {
		result = append(result, mockCategories[index])
	}
	return result
}

func normalizeConfig(cfg Config) Config {
	if cfg.LookbackMonths <= 0 {
		cfg.LookbackMonths = 6
	}
	if cfg.MinCategories <= 0 {
		cfg.MinCategories = 10
	}
	if cfg.MaxCategories <= 0 {
		cfg.MaxCategories = 20
	}
	if cfg.MinCategories > cfg.MaxCategories {
		cfg.MinCategories, cfg.MaxCategories = cfg.MaxCategories, cfg.MinCategories
	}
	if cfg.MaxCategories > len(mockCategories) {
		cfg.MaxCategories = len(mockCategories)
	}
	if cfg.MinCategories > cfg.MaxCategories {
		cfg.MinCategories = cfg.MaxCategories
	}
	if cfg.MaxDailyExpenses < 0 {
		cfg.MaxDailyExpenses = 0
	}
	if cfg.MaxDailyExpenses == 0 {
		cfg.MaxDailyExpenses = 6
	}
	if cfg.Currency == "" {
		cfg.Currency = "USD"
	}
	return cfg
}

func randomAmount(rng *rand.Rand, min, max float64) float64 {
	value := min + rng.Float64()*(max-min)
	return math.Round(value*100) / 100
}

func dateOnlyUTC(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func stringPtr(value string) *string {
	return &value
}

type mockCategory struct {
	name      string
	color     string
	minAmount float64
	maxAmount float64
	titles    []string
}

var mockCategories = []mockCategory{
	{name: "Продукты", color: "#2563eb", minAmount: 8, maxAmount: 95, titles: []string{"Молоко и хлеб", "Овощи и фрукты", "Покупка продуктов", "Мясо и крупы"}},
	{name: "Кафе и рестораны", color: "#dc2626", minAmount: 12, maxAmount: 120, titles: []string{"Обед в кафе", "Ужин в ресторане", "Кофе с собой", "Доставка еды"}},
	{name: "Транспорт", color: "#16a34a", minAmount: 3, maxAmount: 45, titles: []string{"Такси", "Метро и автобус", "Заправка", "Каршеринг"}},
	{name: "Дом", color: "#9333ea", minAmount: 15, maxAmount: 180, titles: []string{"Товары для дома", "Бытовая химия", "Мелкий ремонт", "Посуда"}},
	{name: "Коммунальные услуги", color: "#ea580c", minAmount: 30, maxAmount: 240, titles: []string{"Электричество", "Вода и отопление", "Интернет дома", "Коммунальный платеж"}},
	{name: "Здоровье", color: "#0891b2", minAmount: 10, maxAmount: 160, titles: []string{"Аптека", "Прием врача", "Витамины", "Анализы"}},
	{name: "Дети", color: "#be123c", minAmount: 8, maxAmount: 140, titles: []string{"Игрушки", "Школьные принадлежности", "Детская одежда", "Кружок для детей"}},
	{name: "Одежда", color: "#4f46e5", minAmount: 20, maxAmount: 220, titles: []string{"Одежда", "Обувь", "Аксессуары", "Химчистка"}},
	{name: "Развлечения", color: "#ca8a04", minAmount: 8, maxAmount: 130, titles: []string{"Кино", "Концерт", "Настольные игры", "Билеты на мероприятие"}},
	{name: "Подарки", color: "#db2777", minAmount: 12, maxAmount: 180, titles: []string{"Подарок друзьям", "Цветы", "Сувениры", "Упаковка подарка"}},
	{name: "Путешествия", color: "#0d9488", minAmount: 25, maxAmount: 350, titles: []string{"Отель", "Билеты", "Экскурсия", "Страховка в поездку"}},
	{name: "Связь", color: "#7c3aed", minAmount: 5, maxAmount: 60, titles: []string{"Мобильная связь", "Подписка на интернет", "Роуминг", "Дополнительный пакет"}},
	{name: "Образование", color: "#0284c7", minAmount: 15, maxAmount: 200, titles: []string{"Курс", "Книги", "Онлайн-обучение", "Репетитор"}},
	{name: "Спорт", color: "#65a30d", minAmount: 10, maxAmount: 150, titles: []string{"Абонемент в зал", "Спортивное питание", "Инвентарь", "Тренировка"}},
	{name: "Красота", color: "#c026d3", minAmount: 12, maxAmount: 170, titles: []string{"Стрижка", "Косметика", "Маникюр", "Уходовые средства"}},
	{name: "Хобби", color: "#d97706", minAmount: 8, maxAmount: 110, titles: []string{"Материалы для хобби", "Творческий набор", "Инструменты для мастерской", "Расходники для увлечения"}},
	{name: "Подписки", color: "#475569", minAmount: 3, maxAmount: 50, titles: []string{"Музыкальная подписка", "Онлайн-кинотеатр", "Облачное хранилище", "Сервисная подписка"}},
	{name: "Электроника", color: "#0f766e", minAmount: 20, maxAmount: 300, titles: []string{"Аксессуары для телефона", "Кабель и зарядка", "Компьютерные товары", "Ремонт техники"}},
	{name: "Автомобиль", color: "#b91c1c", minAmount: 15, maxAmount: 260, titles: []string{"Мойка автомобиля", "Парковка", "Техобслуживание", "Автотовары"}},
	{name: "Прочее", color: "#52525b", minAmount: 5, maxAmount: 100, titles: []string{"Небольшая покупка", "Разовые расходы", "Бытовая мелочь", "Неожиданная трата"}},
}
