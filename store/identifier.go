package store

import (
	"github.com/jackc/pgx/v5/pgtype"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

type Identifier identifier.Identifier

func (i *Identifier) ScanUUID(v pgtype.UUID) error {
	if !v.Valid {
		return errors.New("cannot scan NULL into *identifier.Identifier")
	}

	*i = v.Bytes
	return nil
}

func (i Identifier) UUIDValue() (pgtype.UUID, error) {
	return pgtype.UUID{Bytes: [16]byte(i), Valid: true}, nil
}

func TryWrapIdentifierEncodePlan(value interface{}) (plan pgtype.WrappedEncodePlanNextSetter, nextValue interface{}, ok bool) {
	switch value := value.(type) {
	case identifier.Identifier:
		return &wrapIdentifierEncodePlan{}, Identifier(value), true
	}

	return nil, nil, false
}

type wrapIdentifierEncodePlan struct {
	next pgtype.EncodePlan
}

func (plan *wrapIdentifierEncodePlan) SetNext(next pgtype.EncodePlan) {
	plan.next = next
}

func (plan *wrapIdentifierEncodePlan) Encode(value interface{}, buf []byte) (newBuf []byte, err error) {
	return plan.next.Encode(Identifier(value.(identifier.Identifier)), buf)
}

func TryWrapIdentifierScanPlan(target interface{}) (plan pgtype.WrappedScanPlanNextSetter, nextDst interface{}, ok bool) {
	switch target := target.(type) {
	case *identifier.Identifier:
		return &wrapIdentifierScanPlan{}, (*Identifier)(target), true
	}

	return nil, nil, false
}

type wrapIdentifierScanPlan struct {
	next pgtype.ScanPlan
}

func (plan *wrapIdentifierScanPlan) SetNext(next pgtype.ScanPlan) {
	plan.next = next
}

func (plan *wrapIdentifierScanPlan) Scan(src []byte, dst interface{}) error {
	return plan.next.Scan(src, (*Identifier)(dst.(*identifier.Identifier)))
}

type IdentifierCodec struct {
	pgtype.UUIDCodec
}

func (IdentifierCodec) DecodeValue(tm *pgtype.Map, oid uint32, format int16, src []byte) (interface{}, error) {
	if src == nil {
		return nil, nil
	}

	var target identifier.Identifier
	scanPlan := tm.PlanScan(oid, format, &target)
	if scanPlan == nil {
		return nil, errors.New("PlanScan did not find a plan")
	}

	err := scanPlan.Scan(src, &target)
	if err != nil {
		return nil, err
	}

	return target, nil
}

func RegisterIdentifier(tm *pgtype.Map) {
	tm.TryWrapEncodePlanFuncs = append([]pgtype.TryWrapEncodePlanFunc{TryWrapIdentifierEncodePlan}, tm.TryWrapEncodePlanFuncs...)
	tm.TryWrapScanPlanFuncs = append([]pgtype.TryWrapScanPlanFunc{TryWrapIdentifierScanPlan}, tm.TryWrapScanPlanFuncs...)

	tm.RegisterType(&pgtype.Type{
		Name: "identifier",
		// We currently misuse Identifier PostgreSQL field type for identifiers.
		OID:   pgtype.UUIDOID,
		Codec: IdentifierCodec{},
	})
}
