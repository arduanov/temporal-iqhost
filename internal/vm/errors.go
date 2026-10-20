package vm

// RejectedError означает, что операция нарушает правила состояния или владения.
// Повтор того же шага не устранит причину отклонения.
type RejectedError struct{ Message string }

func (err *RejectedError) Error() string { return err.Message }
func (err *RejectedError) Type() string  { return "Rejected" }
