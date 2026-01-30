package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Role string

const (
	RoleDomainEditor  Role = "DomainEditor"
	RoleDomainManager Role = "DomainManager"
	RoleManager       Role = "Manager"
	RoleAdmin         Role = "Admin"
)

type User struct {
	TelegramID int64    `json:"telegram_id"`
	Name       string   `json:"name"`  // UNIQUE
	RolesRaw   []string `json:"roles"` // persisted
	Roles      map[Role]bool `json:"-"` // runtime
}

func (u User) Has(role Role) bool {
	if u.Roles[RoleAdmin] {
		return true
	}
	return u.Roles[role]
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func normalizeUser(u User) (User, error) {
	u.Name = strings.TrimSpace(u.Name)
	if u.TelegramID == 0 || u.Name == "" {
		return User{}, fmt.Errorf("invalid user: telegram_id and name required")
	}
	u.RolesRaw = uniqueStrings(u.RolesRaw)
	u.Roles = map[Role]bool{}
	for _, r := range u.RolesRaw {
		u.Roles[Role(r)] = true
	}
	return u, nil
}

// ---------------- UsersStore (users.json) ----------------

type UsersStore struct {
	Path string
	mu   sync.Mutex
}

func NewUsersStore(path string) *UsersStore { return &UsersStore{Path: path} }

type usersSnapshot struct {
	ByID   map[int64]User
	ByName map[string]User
	List   []User
}

func (us *UsersStore) loadUnlocked() (*usersSnapshot, error) {
	b, err := os.ReadFile(us.Path)
	if err != nil {
		return nil, err
	}
	var list []User
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}

	byID := make(map[int64]User, len(list))
	byName := make(map[string]User, len(list))
	outList := make([]User, 0, len(list))

	for _, u := range list {
		nu, err := normalizeUser(u)
		if err != nil {
			return nil, err
		}
		// уникальность telegram_id
		if _, exists := byID[nu.TelegramID]; exists {
			return nil, fmt.Errorf("users.json: duplicate telegram_id %d", nu.TelegramID)
		}
		// уникальность name
		if _, exists := byName[nu.Name]; exists {
			return nil, fmt.Errorf("users.json: duplicate name %q", nu.Name)
		}

		byID[nu.TelegramID] = nu
		byName[nu.Name] = nu
		outList = append(outList, nu)
	}

	// детерминированный порядок
	sort.Slice(outList, func(i, j int) bool {
		return outList[i].Name < outList[j].Name
	})

	return &usersSnapshot{ByID: byID, ByName: byName, List: outList}, nil
}

func (us *UsersStore) saveUnlocked(list []User) error {
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := us.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, us.Path)
}

func (us *UsersStore) GetByID(tgID int64) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	u, ok := snap.ByID[tgID]
	return u, ok, nil
}

func (us *UsersStore) GetByName(name string) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	u, ok := snap.ByName[strings.TrimSpace(name)]
	return u, ok, nil
}

func (us *UsersStore) ListUsers() ([]User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return nil, err
	}
	return snap.List, nil
}

func (us *UsersStore) GrantByName(name string, role Role) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	u, ok := snap.ByName[name]
	if !ok {
		return "", fmt.Errorf("user %q not found", name)
	}

	u.RolesRaw = append(u.RolesRaw, string(role))
	u.RolesRaw = uniqueStrings(u.RolesRaw)
	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	// replace in list
	for i := range snap.List {
		if snap.List[i].Name == name {
			snap.List[i] = nu
			break
		}
	}
	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}
	return fmt.Sprintf("Выдана роль %s пользователю %s (tg_id=%d)", role, nu.Name, nu.TelegramID), nil
}

func (us *UsersStore) RevokeByName(name string, role Role) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	u, ok := snap.ByName[name]
	if !ok {
		return "", fmt.Errorf("user %q not found", name)
	}

	out := make([]string, 0, len(u.RolesRaw))
	for _, r := range u.RolesRaw {
		if Role(r) == role {
			continue
		}
		out = append(out, r)
	}
	u.RolesRaw = uniqueStrings(out)

	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	for i := range snap.List {
		if snap.List[i].Name == name {
			snap.List[i] = nu
			break
		}
	}
	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}
	return fmt.Sprintf("Снята роль %s у пользователя %s (tg_id=%d)", role, nu.Name, nu.TelegramID), nil
}

func (us *UsersStore) Rename(oldName, newName string) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	if oldName == "" || newName == "" {
		return "", fmt.Errorf("old_name and new_name required")
	}
	if oldName == newName {
		return "", fmt.Errorf("new_name equals old_name")
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	u, ok := snap.ByName[oldName]
	if !ok {
		return "", fmt.Errorf("user %q not found", oldName)
	}
	if _, exists := snap.ByName[newName]; exists {
		return "", fmt.Errorf("name %q already exists", newName)
	}

	// update name
	u.Name = newName
	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	// replace in list
	for i := range snap.List {
		if snap.List[i].Name == oldName {
			snap.List[i] = nu
			break
		}
	}

	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}

	return fmt.Sprintf("Переименован пользователь %q → %q (tg_id=%d)", oldName, newName, nu.TelegramID), nil
}


// ---------------- Domains store ----------------

type Store struct {
	Path string
	mu   sync.Mutex
}

func NewStore(path string) *Store { return &Store{Path: path} }

func (s *Store) Load() (map[string][]string, error) {
	sections := map[string][]string{}

	f, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return sections, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var current string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			current = name
			if _, ok := sections[current]; !ok {
				sections[current] = []string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		sections[current] = append(sections[current], line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	for k, v := range sections {
		sections[k] = normalizeList(v)
	}
	return sections, nil
}

func (s *Store) Save(sections map[string][]string) error {
	tmp := s.Path + ".tmp"

	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)

	for i, k := range keys {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "# %s\n", k)
		list := normalizeList(sections[k])
		for _, d := range list {
			fmt.Fprintln(w, d)
		}
	}

	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *Store) AddDomain(sectionName, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return false, err
	}
	list := sections[sectionName]
	for _, d := range list {
		if d == domain {
			return false, nil
		}
	}
	sections[sectionName] = append(list, domain)
	sections[sectionName] = normalizeList(sections[sectionName])
	return true, s.Save(sections)
}

func (s *Store) DelDomain(sectionName, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return false, err
	}
	list, ok := sections[sectionName]
	if !ok {
		return false, nil
	}
	out := make([]string, 0, len(list))
	removed := false
	for _, d := range list {
		if d == domain {
			removed = true
			continue
		}
		out = append(out, d)
	}
	if !removed {
		return false, nil
	}
	sections[sectionName] = normalizeList(out)
	return true, s.Save(sections)
}

func (s *Store) List(sectionName string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return nil, err
	}
	return normalizeList(sections[sectionName]), nil
}

func (s *Store) ExportAll() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# %s\n", k)
		for _, d := range normalizeList(sections[k]) {
			b.WriteString(d)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

type ContainmentMatch struct {
	ParentDomain  string
	ParentSection string
}

func (s *Store) FindLongestContainingDomain(candidate string) (*ContainmentMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return nil, err
	}

	var best *ContainmentMatch
	for sec, list := range sections {
		for _, parent := range list {
			if parent == candidate {
				continue
			}
			if isSubdomainOf(candidate, parent) {
				if best == nil || len(parent) > len(best.ParentDomain) {
					best = &ContainmentMatch{ParentDomain: parent, ParentSection: sec}
				}
			}
		}
	}
	return best, nil
}

func (s *Store) ReplaceDomain(parentSection, parentDomain, targetSection, candidate string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return err
	}

	// remove parent
	list := sections[parentSection]
	newList := make([]string, 0, len(list))
	for _, d := range list {
		if d == parentDomain {
			continue
		}
		newList = append(newList, d)
	}
	sections[parentSection] = normalizeList(newList)

	// add candidate
	sections[targetSection] = append(sections[targetSection], candidate)
	sections[targetSection] = normalizeList(sections[targetSection])

	return s.Save(sections)
}

func (s *Store) RenameSection(oldName, newName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return "", fmt.Errorf("old_name and new_name required")
	}
	if oldName == newName {
		return "Секция не изменена (имена совпадают).", nil
	}

	sections, err := s.Load()
	if err != nil {
		return "", err
	}

	oldList, ok := sections[oldName]
	if !ok {
		// секции могло и не быть — это не фатально
		return fmt.Sprintf("Секция #%s не найдена — нечего переименовывать.", oldName), nil
	}

	// merge if target exists
	if newList, exists := sections[newName]; exists {
		merged := append(newList, oldList...)
		sections[newName] = normalizeList(merged)
	} else {
		sections[newName] = normalizeList(oldList)
	}

	delete(sections, oldName)

	if err := s.Save(sections); err != nil {
		return "", err
	}

	return fmt.Sprintf("Секция доменов переименована #%s → #%s", oldName, newName), nil
}


// ---------------- Domain normalization (ONLY domains; strip ":" and port) ----------------

var reDomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func normalizeDomain(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.ToLower(s)

	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")

	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}

	// only domains, no ports
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}

	s = strings.TrimPrefix(s, ".")
	s = strings.TrimSuffix(s, ".")

	if s == "" {
		return "", errors.New("пустой домен")
	}
	if len(s) > 253 {
		return "", errors.New("слишком длинный домен")
	}
	if !reDomain.MatchString(s) {
		return "", errors.New("не похоже на домен (пример: example.com)")
	}
	return s, nil
}

func normalizeList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func isSubdomainOf(sub, parent string) bool {
	if len(sub) <= len(parent) {
		return false
	}
	if !strings.HasSuffix(sub, parent) {
		return false
	}
	idx := len(sub) - len(parent) - 1
	return idx >= 0 && sub[idx] == '.'
}

// ---------------- Manager: run command inside swarm service container ----------------

func getTargetService() string { return strings.TrimSpace(os.Getenv("TARGET_SWARM_SERVICE")) }
func getManagerCmd() string    { return strings.TrimSpace(os.Getenv("MANAGER_BASH_CMD")) }

func runInSwarmServiceContainer(serviceName, bashCmd string) (string, error) {
	if serviceName == "" {
		return "", errors.New("TARGET_SWARM_SERVICE не задан")
	}
	if bashCmd == "" {
		return "", errors.New("MANAGER_BASH_CMD не задан")
	}

	ps := exec.Command("docker", "ps",
		"--filter", "label=com.docker.swarm.service.name="+serviceName,
		"--format", "{{.ID}}",
	)

	var psOut, psErr bytes.Buffer
	ps.Stdout = &psOut
	ps.Stderr = &psErr
	if err := ps.Run(); err != nil {
		return "", fmt.Errorf("docker ps error: %v: %s", err, strings.TrimSpace(psErr.String()))
	}

	ids := strings.Fields(psOut.String())
	if len(ids) == 0 {
		return "", fmt.Errorf("не найден ни один контейнер сервиса %q", serviceName)
	}
	containerID := ids[0]

	ex := exec.Command("docker", "exec", containerID, "bash", "-lc", bashCmd)
	var out, errb bytes.Buffer
	ex.Stdout = &out
	ex.Stderr = &errb

	if err := ex.Run(); err != nil {
		return strings.TrimSpace(out.String()) + "\n" + strings.TrimSpace(errb.String()),
			fmt.Errorf("docker exec failed: %v", err)
	}

	combined := strings.TrimSpace(out.String())
	if e := strings.TrimSpace(errb.String()); e != "" {
		combined = strings.TrimSpace(combined + "\n" + e)
	}
	return combined, nil
}

// ---------------- Pending add state + callbacks (inline buttons) ----------------

type PendingAdd struct {
	Candidate     string
	ParentDomain  string
	ParentSection string
	TargetSection string
	ChatID        int64
	MessageID     int
}

var pendingMu sync.Mutex
var pending = map[int64]PendingAdd{} // key: TelegramID

func setPending(tgID int64, p PendingAdd) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pending[tgID] = p
}
func getPending(tgID int64) (PendingAdd, bool) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	p, ok := pending[tgID]
	return p, ok
}
func clearPending(tgID int64) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	delete(pending, tgID)
}

const (
	cbAddReplace = "add:replace"
	cbAddSub     = "add:sub"
	cbAddCancel  = "add:cancel"
)

// ---------------- Help ----------------

const helpText = `Команды:
/add <domain>      — добавить домен в твою секцию (DomainEditor)
/del <domain>      — удалить домен из твоей секции (DomainEditor)
/list              — показать домены твоей секции (DomainEditor)
/export            — экспорт всего списка (DomainManager)
/manage            — выполнить команду в контейнере сервиса (Manager)

/users             — список пользователей (Admin)
/grant <name> <role>   — выдать роль (Admin)
/revoke <name> <role>  — снять роль (Admin)
/roles [name]      — роли (Admin может смотреть чужие)

/help              — помощь
`

func main() {
//    godotenv.Load()
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("set TELEGRAM_BOT_TOKEN")
	}

	usersFile := os.Getenv("USERS_FILE")
	if usersFile == "" {
		usersFile = "./users.json"
	}
	usersStore := NewUsersStore(usersFile)

	domainsPath := os.Getenv("DOMAINS_FILE")
	if domainsPath == "" {
		domainsPath = "./domains.txt"
	}
	store := NewStore(domainsPath)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = false

	log.Printf("Bot authorized as @%s", bot.Self.UserName)
	log.Printf("Users file: %s", usersFile)
	log.Printf("Domains file: %s", domainsPath)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			handleCallback(bot, usersStore, store, update.CallbackQuery)
			continue
		}

		if update.Message == nil || update.Message.From == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		tgID := update.Message.From.ID

		user, ok, err := usersStore.GetByID(tgID)
		if err != nil {
			reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
			continue
		}
		if !ok {
			reply(bot, chatID, "Доступ запрещён: ты не в списке пользователей.")
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		if text == "" {
			continue
		}
		cmd, arg := splitCmd(text)

		switch cmd {
		case "/start", "/help", "help":
			reply(bot, chatID, helpText)

		case "/users", "users":
			if !user.Has(RoleAdmin) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
				continue
			}
			list, err := usersStore.ListUsers()
			if err != nil {
				reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
				continue
			}
			reply(bot, chatID, formatUsers(list))

		case "/roles", "roles":
			targetName := strings.TrimSpace(arg)
			if targetName == "" {
				// свои
				reply(bot, chatID, fmt.Sprintf("Пользователь %s (tg_id=%d)\nРоли: %s",
					user.Name, user.TelegramID, strings.Join(uniqueStrings(user.RolesRaw), ", "),
				))
				continue
			}
			if !user.Has(RoleAdmin) {
				reply(bot, chatID, "Недостаточно прав. Смотреть роли других может только Admin.")
				continue
			}
			tu, ok, err := usersStore.GetByName(targetName)
			if err != nil {
				reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
				continue
			}
			if !ok {
				reply(bot, chatID, "Пользователь не найден: "+targetName)
				continue
			}
			reply(bot, chatID, fmt.Sprintf("Пользователь %s (tg_id=%d)\nРоли: %s",
				tu.Name, tu.TelegramID, strings.Join(uniqueStrings(tu.RolesRaw), ", "),
			))

		case "/grant", "grant":
			if !user.Has(RoleAdmin) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
				continue
			}
			fields := strings.Fields(arg)
			if len(fields) != 2 {
				reply(bot, chatID, "Формат: /grant <name> <role>")
				continue
			}
			name := fields[0]
			role := Role(fields[1])
			msg, err := usersStore.GrantByName(name, role)
			if err != nil {
				reply(bot, chatID, "Ошибка: "+err.Error())
				continue
			}
			reply(bot, chatID, msg)

		case "/revoke", "revoke":
			if !user.Has(RoleAdmin) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
				continue
			}
			fields := strings.Fields(arg)
			if len(fields) != 2 {
				reply(bot, chatID, "Формат: /revoke <name> <role>")
				continue
			}
			name := fields[0]
			role := Role(fields[1])
			msg, err := usersStore.RevokeByName(name, role)
			if err != nil {
				reply(bot, chatID, "Ошибка: "+err.Error())
				continue
			}
			reply(bot, chatID, msg)

		case "/add", "add":
			if !user.Has(RoleDomainEditor) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).")
				continue
			}
			if arg == "" {
				reply(bot, chatID, "Укажи домен: /add example.com")
				continue
			}
			d, err := normalizeDomain(arg)
			if err != nil {
				reply(bot, chatID, "Ошибка: "+err.Error())
				continue
			}

			match, err := store.FindLongestContainingDomain(d)
			if err != nil {
				reply(bot, chatID, "Ошибка чтения списка: "+err.Error())
				continue
			}

			if match != nil {
				msgText := fmt.Sprintf(
					"Домен *%s* является поддоменом домена *%s* (секция: #%s).\nВыбери действие:",
					d, match.ParentDomain, match.ParentSection,
				)

				canReplace := (match.ParentSection == user.Name) || user.Has(RoleDomainManager)
				canSub := user.Has(RoleDomainManager)

				kb := buildAddDecisionKeyboard(canReplace, canSub)
				m := tgbotapi.NewMessage(chatID, msgText)
				m.ParseMode = "Markdown"
				m.ReplyMarkup = kb
				sent, err := bot.Send(m)
				if err != nil {
					reply(bot, chatID, "Ошибка отправки: "+err.Error())
					continue
				}

				setPending(tgID, PendingAdd{
					Candidate:     d,
					ParentDomain:  match.ParentDomain,
					ParentSection: match.ParentSection,
					TargetSection: user.Name,
					ChatID:        chatID,
					MessageID:     sent.MessageID,
				})
				continue
			}

			added, err := store.AddDomain(user.Name, d)
			if err != nil {
				reply(bot, chatID, "Ошибка сохранения: "+err.Error())
				continue
			}
			if !added {
				reply(bot, chatID, "Уже есть в твоей секции: "+d)
				continue
			}
			reply(bot, chatID, "Добавлено в секцию #"+user.Name+": "+d)

		case "/del", "del":
			if !user.Has(RoleDomainEditor) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).")
				continue
			}
			if arg == "" {
				reply(bot, chatID, "Укажи домен: /del example.com")
				continue
			}
			d, err := normalizeDomain(arg)
			if err != nil {
				reply(bot, chatID, "Ошибка: "+err.Error())
				continue
			}
			removed, err := store.DelDomain(user.Name, d)
			if err != nil {
				reply(bot, chatID, "Ошибка сохранения: "+err.Error())
				continue
			}
			if !removed {
				reply(bot, chatID, "Не найдено в твоей секции: "+d)
				continue
			}
			reply(bot, chatID, "Удалено из секции #"+user.Name+": "+d)

		case "/list", "list":
			if !user.Has(RoleDomainEditor) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).")
				continue
			}
			list, err := store.List(user.Name)
			if err != nil {
				reply(bot, chatID, "Ошибка чтения: "+err.Error())
				continue
			}
			if len(list) == 0 {
				reply(bot, chatID, "В твоей секции #"+user.Name+" пока нет доменов.")
				continue
			}
			reply(bot, chatID, formatSection(user.Name, list))

		case "/export", "export":
			if !user.Has(RoleDomainManager) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль DomainManager (или Admin).")
				continue
			}
			all, err := store.ExportAll()
			if err != nil {
				reply(bot, chatID, "Ошибка экспорта: "+err.Error())
				continue
			}
			if strings.TrimSpace(all) == "" {
				reply(bot, chatID, "Файл пуст.")
				continue
			}
			reply(bot, chatID, all)

		case "/manage", "manage":
			if !user.Has(RoleManager) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль Manager (или Admin).")
				continue
			}
			out, err := runInSwarmServiceContainer(getTargetService(), getManagerCmd())
			if err != nil {
				reply(bot, chatID, "Команда выполнена с ошибкой:\n"+truncate(out, 3500)+"\n\n"+err.Error())
				continue
			}
			if strings.TrimSpace(out) == "" {
				out = "(пустой вывод)"
			}
			reply(bot, chatID, "OK. Вывод:\n"+truncate(out, 3800))

		case "/rename", "rename":
			if !user.Has(RoleAdmin) {
				reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
				continue
			}
			fields := strings.Fields(arg)
			if len(fields) != 2 {
				reply(bot, chatID, "Формат: /rename <old_name> <new_name>")
				continue
			}
			oldName := fields[0]
			newName := fields[1]

			// 1) users.json
			msg1, err := usersStore.Rename(oldName, newName)
			if err != nil {
				reply(bot, chatID, "Ошибка: "+err.Error())
				continue
			}

			// 2) domains file: rename section header
			msg2, err := store.RenameSection(oldName, newName)
			if err != nil {
				reply(bot, chatID, msg1+"\n\nОшибка при переименовании секции доменов: "+err.Error())
				continue
			}

			reply(bot, chatID, msg1+"\n"+msg2)

		default:
			reply(bot, chatID, "Не понял команду. /help")
		}
	}
}

func handleCallback(bot *tgbotapi.BotAPI, usersStore *UsersStore, store *Store, q *tgbotapi.CallbackQuery) {
	ack := tgbotapi.NewCallback(q.ID, "")
	_, _ = bot.Request(ack)

	if q.From == nil {
		return
	}
	chatID := q.Message.Chat.ID
	tgID := q.From.ID

	user, ok, err := usersStore.GetByID(tgID)
	if err != nil {
		reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(bot, chatID, "Доступ запрещён: ты не в списке пользователей.")
		return
	}

	p, ok := getPending(tgID)
	if !ok {
		reply(bot, chatID, "Нет ожидающего добавления. Используй /add <domain>.")
		return
	}
	if q.Message != nil && (q.Message.MessageID != p.MessageID || q.Message.Chat.ID != p.ChatID) {
		reply(bot, chatID, "Эта кнопка уже устарела. Повтори /add <domain>.")
		return
	}

	switch q.Data {
	case cbAddCancel:
		clearPending(tgID)
		editMessage(bot, chatID, p.MessageID, "Ок, отменил.", nil)

	case cbAddReplace:
		if !user.Has(RoleDomainEditor) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).", nil)
			clearPending(tgID)
			return
		}
		canReplace := (p.ParentSection == user.Name) || user.Has(RoleDomainManager)
		if !canReplace {
			editMessage(bot, chatID, p.MessageID, "Нельзя: покрывающий домен в чужой секции, а роли DomainManager/Admin нет.", nil)
			return
		}
		if err := store.ReplaceDomain(p.ParentSection, p.ParentDomain, p.TargetSection, p.Candidate); err != nil {
			editMessage(bot, chatID, p.MessageID, "Ошибка сохранения: "+err.Error(), nil)
			return
		}
		clearPending(tgID)
		editMessage(bot, chatID, p.MessageID,
			fmt.Sprintf("OK: удалил *%s* из #%s и добавил *%s* в #%s",
				p.ParentDomain, p.ParentSection, p.Candidate, p.TargetSection),
			nil,
		)

	case cbAddSub:
		if !user.Has(RoleDomainManager) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainManager (или Admin).", nil)
			return
		}
		added, err := store.AddDomain(p.TargetSection, p.Candidate)
		if err != nil {
			editMessage(bot, chatID, p.MessageID, "Ошибка сохранения: "+err.Error(), nil)
			return
		}
		clearPending(tgID)
		if !added {
			editMessage(bot, chatID, p.MessageID, "Уже есть в твоей секции: "+p.Candidate, nil)
			return
		}
		editMessage(bot, chatID, p.MessageID,
			fmt.Sprintf("OK: добавил поддомен *%s* в #%s (домен *%s* в #%s не трогал)",
				p.Candidate, p.TargetSection, p.ParentDomain, p.ParentSection),
			nil,
		)

	default:
		reply(bot, chatID, "Неизвестное действие.")
	}
}

func buildAddDecisionKeyboard(canReplace, canSub bool) tgbotapi.InlineKeyboardMarkup {
	var row []tgbotapi.InlineKeyboardButton
	if canReplace {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("➕ Заменить (удалить домен)", cbAddReplace))
	}
	if canSub {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("➕ Добавить поддомен (не удалять)", cbAddSub))
	}
	cancelRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", cbAddCancel),
	}
	kb := [][]tgbotapi.InlineKeyboardButton{}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	kb = append(kb, cancelRow)
	return tgbotapi.NewInlineKeyboardMarkup(kb...)
}

func editMessage(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, markup *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, _ = bot.Send(edit)
}

func reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, _ = bot.Send(msg)
}

func splitCmd(s string) (cmd, arg string) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", ""
	}
	cmd = parts[0]
	if strings.Contains(cmd, "@") {
		cmd = strings.SplitN(cmd, "@", 2)[0]
	}
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
		arg = strings.TrimSpace(arg)
	}
	return cmd, arg
}

func formatSection(section string, domains []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", section)
	for _, d := range domains {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\n(%d домен(ов), %s)", len(domains), time.Now().Format("2006-01-02 15:04:05"))
	return b.String()
}

func formatUsers(list []User) string {
	var b strings.Builder
	b.WriteString("Пользователи:\n")
	for _, u := range list {
		roles := strings.Join(uniqueStrings(u.RolesRaw), ", ")
		if roles == "" {
			roles = "(нет)"
		}
		fmt.Fprintf(&b, "• %s — tg_id=%d — %s\n", u.Name, u.TelegramID, roles)
	}
	return b.String()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

// (оставил импорт strconv выше — на случай расширений, сейчас он не обязателен)
// чтобы не ругался компилятор, используем strconv здесь минимально:
var _ = strconv.IntSize
