package export

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/identifier"
)

//nolint:gochecknoglobals
var (
	TestingResolveDiagramRefTargets = resolveDiagramRefTargets
	TestingDiagramValuesTag         = diagramValuesTag
	TestingClassifyDiagramValueType = classifyDiagramValueType
	TestingCardinalityRightSymbol   = cardinalityRightSymbol
	TestingCardinalityLabel         = cardinalityLabel
	TestingResolveDiagramShortcutID = resolveDiagramShortcutID
	TestingExtractDiagramClassInfo  = extractDiagramClassInfo
	TestingEmbedsStruct             = embedsStruct
	TestingValidateDiagramTypes     = validateDiagramTypes
)

// TestingWalkSubFields invokes walkSubFields and returns its emitted rows and
// relations as deterministic strings so external tests don't need to touch
// the unexported diagramFieldRow/diagramRelation types.
func TestingWalkSubFields(
	entityName, parentMnemonic string,
	t reflect.Type,
	idToName map[identifier.Identifier]string,
	logger zerolog.Logger,
) ([]string, []string) {
	var collectedRows []diagramFieldRow
	var collectedRelations []diagramRelation
	walkSubFields(logger, entityName, parentMnemonic, t, idToName, &collectedRows, &collectedRelations, map[reflect.Type]bool{}, nil)

	rows := make([]string, 0, len(collectedRows))
	for _, r := range collectedRows {
		flag := ""
		if len(r.flags) > 0 {
			flag = " " + strings.Join(r.flags, ",")
		}
		rows = append(rows, fmt.Sprintf("%s %s%s %q", r.valueType, r.name, flag, r.comment))
	}
	relations := make([]string, 0, len(collectedRelations))
	for _, r := range collectedRelations {
		sep := "--"
		if r.dashed {
			sep = ".."
		}
		relations = append(relations, fmt.Sprintf("%s %s%s%s %s : %q", r.source, r.cardLeft, sep, r.cardRight, r.target, r.label))
	}
	return rows, relations
}
