package locale

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"qshqn/core/fiox"
	"qshqn/core/qsh"
	"qshqn/core/typex"
	"qshqn/core/util/stringx"
)

const (
	DEFAULT_LOCALE_PATH_FORMAT = "data/locale/%s/data.json"
	ARRAY_SEPARATOR            = "|"
	SUB_ID_SEPARATOR           = "."
	ROOT_TEMPLATE_NODE         = "root"
	NOUN_FUNC_NAME             = "noun"

	LOCALE_FOLDER = "locale"
	SHARED_FOLDER = "shared"
)

var (
	locales          = map[LangCode]Locale{}
	typeForMsgID     = reflect.TypeFor[MsgID]()
	templateDataPool = typex.NewPool(
		func() map[string]string {
			return make(map[string]string, 8)
		},
	)
	pathRegex   = regexp.MustCompile(`\{\{path:(.+?)\}\}`)
	sharedRegex = regexp.MustCompile(`\{\{shared:(.+?)\}\}`)
	msgRegex    = regexp.MustCompile(`\{\{msg:(.+?)\}\}`)

	localeRootDir string
	sharedDir     string
)

type KVPair struct {
	K string
	V any
}

type MsgMap map[MsgID]string

type Locale struct {
	Path      string
	Map       MsgMap
	Templates *template.Template
}

type IDs interface {
	AsMsgID() MsgID
}
type MissingMsgIDs map[LangCode][]MsgID

type Resolvable interface {
	Resolve(code LangCode) string
}

type AllIDs interface {
	AllIDs(code LangCode) []MsgID
}

func resolvePlaceholderFile(re *regexp.Regexp, rootDir, dirType, keyPath, text string) (string, error) {
	if !re.MatchString(text) {
		return text, nil
	}

	var errs []error
	newText := re.ReplaceAllStringFunc(text, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		filePath := submatches[1]
		filePath = strings.ReplaceAll(filePath, "*", keyPath)
		if filepath.IsAbs(filePath) {
			errs = append(errs, fmt.Errorf("[%s]: absolute paths are not allowed", filePath))
			return match
		}

		targetPath := filepath.Join(rootDir, filePath)

		rel, relErr := filepath.Rel(rootDir, targetPath)
		if relErr != nil || strings.HasPrefix(rel, "..") {
			errs = append(errs, fmt.Errorf("[%s]: path escapes the %s directory", filePath, dirType))
			return match
		}

		content, loadErr := loadAssetString(targetPath)
		if loadErr != nil {
			errs = append(errs, fmt.Errorf("[%s]: %w", targetPath, loadErr))
			return match
		}
		return content
	})

	return newText, errors.Join(errs...)
}

func resolvePathPlaceholder(baseDir, keyPath, text string) (string, error) {
	return resolvePlaceholderFile(pathRegex, baseDir, LOCALE_FOLDER, keyPath, text)
}

func resolveSharedPlaceholder(keyPath, text string) (string, error) {
	return resolvePlaceholderFile(sharedRegex, sharedDir, SHARED_FOLDER, keyPath, text)
}

func resolveMsgPlaceholders(rawJSON map[MsgID]string) (map[MsgID]string, []error) {
	var errs []error
	resolved := make(map[MsgID]string, len(rawJSON))
	maps.Copy(resolved, rawJSON)

	var resolve func(id MsgID, visited map[MsgID]bool) (string, error)
	resolve = func(id MsgID, visited map[MsgID]bool) (string, error) {
		text, exists := rawJSON[id]
		if !exists {
			return "", fmt.Errorf("references non-existent key [%s]", id)
		}
		if visited[id] {
			return "", fmt.Errorf("circular reference detected at [%s]", id)
		}
		if !msgRegex.MatchString(text) {
			return text, nil
		}

		visited[id] = true
		defer func() { visited[id] = false }()

		var err error
		newText := msgRegex.ReplaceAllStringFunc(text, func(match string) string {
			submatches := msgRegex.FindStringSubmatch(match)
			refID := MsgID(submatches[1])
			replacement, refErr := resolve(refID, visited)
			if refErr != nil {
				err = fmt.Errorf("in [%s]: %w", refID, refErr)
				return match
			}
			return replacement
		})
		if err != nil {
			return "", err
		}
		return newText, nil
	}

	for id := range rawJSON {
		visited := make(map[MsgID]bool)
		text, err := resolve(id, visited)
		if err != nil {
			errs = append(errs, fmt.Errorf("msg[%s]: %w", id, err))
		} else {
			resolved[id] = text
		}
	}

	return resolved, errs
}

func parseLocaleData(path string, rawJSON map[MsgID]string) (Locale, []error) {
	var errs []error
	loc := Locale{
		Path: path,
		Map:  make(map[MsgID]string),
	}

	loc.Templates = template.New(ROOT_TEMPLATE_NODE).Funcs(template.FuncMap{
		NOUN_FUNC_NAME: func(code LangCode, baseID string, caseName string, amount int) string {
			lang := code.Lang()
			form := lang.Pluralizer.Func(amount)

			caseID := MakeMsgID(baseID, caseName)
			formID := MakeMsgID(caseID, form)

			return Msg(lang.Code, formID)
		},
	})

	baseDir := filepath.Dir(path)

	pathResolved := make(map[MsgID]string, len(rawJSON))
	for ID, text := range rawJSON {
		keyPath := strings.ReplaceAll(ID.String(), ".", "/")

		resolved, err := resolvePathPlaceholder(baseDir, keyPath, text)
		if err != nil {
			errs = append(errs, fmt.Errorf("msg[%s]: %w", ID, err))
			continue
		}

		resolved, err = resolveSharedPlaceholder(keyPath, resolved)
		if err != nil {
			errs = append(errs, fmt.Errorf("msg[%s]: %w", ID, err))
			continue
		}

		pathResolved[ID] = resolved
	}
	if len(errs) > 0 {
		return loc, errs
	}

	finalResolved, msgErrs := resolveMsgPlaceholders(pathResolved)
	if len(msgErrs) > 0 {
		return loc, msgErrs
	}

	for ID, resolved := range finalResolved {
		if strings.Contains(resolved, "{{") {
			template.Must(loc.Templates.New(ID.String()).Parse(resolved))
		} else {
			loc.Map[ID] = resolved
		}
	}

	return loc, nil
}

func getMsg(code LangCode, ID MsgID) (string, error) {
	if langData, ok := locales[code]; ok {
		if m, ok := langData.Map[ID]; ok {
			return m, nil
		}
	}

	return "", fmt.Errorf("!MSG_MISSING[%s:%s]!", code, ID)
}

func asVariable(value string) string {
	return "{{." + value + "}}"
}

func KV(k string, v any) KVPair {
	return KVPair{K: k, V: v}
}

// (~"id", ~"sub_id") -> ~"id.sub_id" for separator = "."
func MakeID[Return, S1, S2 ~string](ID S1, SubID S2) Return {
	return Return(string(ID) + SUB_ID_SEPARATOR + string(SubID))
}

// ("id", "sub_id") -> "id.sub_id" for separator = "."
func MakeMsgID[S1, S2 ~string](ID S1, SubID S2) MsgID {
	return MakeID[MsgID](ID, SubID)
}

// returns message; on error returns empty string
func MsgRaw(code LangCode, ID MsgID) string {
	msg, _ := getMsg(code, ID)
	return msg
}

// returns message and error if any
func MsgSafe(code LangCode, ID MsgID) (string, error) {
	return getMsg(code, ID)
}

// returns message; on error returns error string
func Msg(code LangCode, ID MsgID) string {
	template, err := getMsg(code, ID)
	if err != nil {
		return err.Error()
	}
	return template
}

func Msgf(code LangCode, ID MsgID, pairs ...KVPair) string {
	loc, ok := locales[code]
	if !ok {
		return fmt.Sprintf("!LANG_MISSING[%s]!", code)
	}

	tmpl := loc.Templates.Lookup(ID.String())
	if tmpl == nil {
		msg, err := getMsg(code, ID)
		if err != nil {
			return err.Error()
		}
		return msg
	}

	data := templateDataPool.Get()
	defer func() {
		clear(data)
		templateDataPool.Put(data)
	}()

	for _, p := range pairs {
		var vStr string
		switch r := p.V.(type) {
		case Resolvable:
			vStr = r.Resolve(code)
		default:
			vStr = string(String(p.V))
		}
		data[p.K] = vStr
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		qsh.Errorf("!TEMPLATE_EXEC_ERROR[%s:%s]!: %w", code, ID, err)
		return fmt.Sprintf("!TEMPLATE_EXEC_ERROR[%s:%s]!", code, ID)
	}
	return sb.String()
}

func MsgExistsFor(code LangCode, ID MsgID) bool {
	data, ok := locales[code]
	if !ok {
		panic(fmt.Errorf("wanted to check if msg[%s] exists for invalid lang[%s]", ID, code))
	}
	_, ok = data.Map[ID]
	return ok
}

func DefaultLocalePathFromCode(code LangCode) string {
	return fmt.Sprintf(DEFAULT_LOCALE_PATH_FORMAT, code)
}

func FillMsgIDs[T any](ID string, otherMsgsPtr *T) error {
	if otherMsgsPtr == nil {
		return fmt.Errorf("otherMsgsPtr is nil")
	}

	val := reflect.ValueOf(otherMsgsPtr).Elem()
	typ := val.Type()

	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("otherMsgsPtr is not a struct")
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		if field.Type == typeForMsgID && fieldVal.CanSet() {
			if fieldVal.String() == "" {
				subID := field.Tag.Get("loc")
				if subID == "" {
					subID = stringx.ToSnakeCase(field.Name)
				}

				fullID := MakeMsgID(ID, subID)
				fieldVal.SetString(string(fullID))
			}
		}
	}

	return nil
}

func VerifyMsgIDs[T AllIDs](msgIDs []T) (missing MissingMsgIDs, err error) {
	missing = MissingMsgIDs{}
	for code, lang := range langs {
		for _, provider := range msgIDs {
			for _, id := range provider.AllIDs(code) {
				loc := locales[lang.Code]
				_, inMap := loc.Map[id]
				inTemplates := loc.Templates.Lookup(string(id)) != nil
				if !inMap && !inTemplates {
					missing[code] = append(missing[code], id)
				}
			}
		}
	}

	if len(missing) != 0 {
		var sb strings.Builder
		for code, ids := range missing {
			fmt.Fprintf(&sb, "- [%s]:\n", code)
			for _, id := range ids {
				fmt.Fprintf(&sb, "  - [%s]\n", id)
			}
		}

		err = fmt.Errorf("missing msg ids:\n%s", sb.String())
	}

	return missing, err
}

func CollectMsgIDs(v any) []AllIDs {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Struct {
		panic(fmt.Sprintf("tried to collect msg ids of non-struct v=%T", v))
	}

	var ids []AllIDs
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if provider, ok := f.Interface().(AllIDs); ok {
			ids = append(ids, provider)
			continue
		}

		if f.Kind() == reflect.Struct {
			ids = append(ids, CollectMsgIDs(f.Interface())...)
		}
	}

	return ids
}

func escapeJSONString(s string) string {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSpace(buf.String())
}

func SaveLocaleDataPretty(path string, data MsgMap) error {
	keys := slices.Collect(maps.Keys(data))

	prefixWeight := func(prefix string) int {
		switch prefix {
		case RootIDGlobal:
			return 1
		case RootIDTalk:
			return 2
		default:
			return 99
		}
	}

	slices.SortFunc(keys, func(a, b MsgID) int {
		p1, _, _ := strings.Cut(string(a), SUB_ID_SEPARATOR)
		p2, _, _ := strings.Cut(string(b), SUB_ID_SEPARATOR)

		if w1, w2 := prefixWeight(p1), prefixWeight(p2); w1 != w2 {
			return cmp.Compare(w1, w2)
		}
		return cmp.Compare(a, b)
	})

	var sb strings.Builder
	sb.WriteString("{\n")

	var lastPrefix string
	for i, k := range keys {
		parts := strings.SplitN(string(k), SUB_ID_SEPARATOR, 2)
		prefix := parts[0]

		if i > 0 {
			if prefix != lastPrefix {
				sb.WriteString(",\n\n")
			} else {
				sb.WriteString(",\n")
			}
		}
		lastPrefix = prefix

		keyJSON := escapeJSONString(string(k))
		valJSON := escapeJSONString(data[MsgID(k)])

		fmt.Fprintf(&sb, "    %s: %s", keyJSON, valJSON)
	}

	sb.WriteString("\n}\n")

	return fiox.SafeWrite(path, sb.String(), func(w io.Writer, data any) error {
		_, err := w.Write([]byte(data.(string)))
		return err
	})
}

func ResolveMissingMsgIDs(missing MissingMsgIDs) error {
	if len(missing) == 0 {
		return nil
	}

	confirm1, err := qsh.YesNo("add them?")
	if err != nil {
		return fmt.Errorf("input error when asking for first confirmation to add missing msg ids: %w", err)
	}
	if !confirm1 {
		return fmt.Errorf("missing msg ids and declined to add")
	}

	confirm2, err := qsh.YesNo("are you sure?")
	if err != nil {
		return fmt.Errorf("input error when asking for second confirmation to add missing msg ids: %w", err)
	}
	if !confirm2 {
		return fmt.Errorf("missing msg ids and declined to add")
	}

	const missingValue = "TODO"
	qsh.Info("adding missing msg ids...")

	for code, ids := range missing {
		loc := locales[code]

		rawMap, err := fiox.Load[MsgMap](loc.Path, fiox.NoReadCache, fiox.NoSetCache)
		if err != nil {
			return fmt.Errorf("failed to reload locale [%s] to add keys: %w", code, err)
		}
		if rawMap == nil {
			rawMap = make(MsgMap)
		}

		for _, id := range ids {
			rawMap[id] = missingValue
		}

		if err := SaveLocaleDataPretty(loc.Path, rawMap); err != nil {
			return fmt.Errorf("failed to save formatted locale [%s]: %w", code, err)
		}

		qsh.Infof("locale at [%s] updated", loc.Path)
	}

	return fmt.Errorf("missing msg ids added with value [%s]. fill them in and restart", missingValue)
}

func Init(opts InitOptions) error {
	qsh.Debug("initializing locale module")
	for _, w := range defaultTriggers {
		triggers[w] = struct{}{}
	}

	var samplePath string
	for code := range langs {
		path, specified := opts.LocalePaths[code]
		if !specified {
			path = DefaultLocalePathFromCode(code)
		}
		samplePath = path
		break
	}
	if samplePath != "" {
		localeRootDir = filepath.Dir(filepath.Dir(samplePath))
		sharedDir = filepath.Join(localeRootDir, SHARED_FOLDER)
	}

	var localeErrs []error
	for code := range langs {
		path, specified := opts.LocalePaths[code]
		if !specified {
			path = DefaultLocalePathFromCode(code)
		}

		contents, err := loadAssetMsgMap(path)
		if err != nil {
			return fmt.Errorf("error initializing locale for lang code [%s] at path [%s]: %w", code, path, err)
		}

		loc, errs := parseLocaleData(path, contents)
		if len(errs) > 0 {
			localeErrs = append(localeErrs, errs...)
			continue
		}
		locales[code] = loc
	}

	if len(localeErrs) > 0 {
		return fmt.Errorf("locale initialization failed:\n%w", errors.Join(localeErrs...))
	}

	if missing, err := VerifyMsgIDs(CollectMsgIDs(GlobalIDs)); err != nil {
		qsh.Error(err)
		return ResolveMissingMsgIDs(missing)
	}

	return nil
}
