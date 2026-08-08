package db

// ledgerWriteHook, quando definido, é chamado de forma não bloqueante toda vez
// que uma escrita no ledger é confirmada com sucesso via AggregateRepository.Save
// ou SaveWithAudit. Existe para acordar o Projection Manager sem que o pacote
// db precise importar o pacote projections (evitaria import cycle, já que
// projections importa db). Definido uma única vez em cmd/server/main.go,
// antes de router.Run(), a partir de projManager.Wake.
var ledgerWriteHook func()

// SetLedgerWriteHook registra o callback chamado após cada escrita confirmada
// no ledger. Passar nil desativa o callback. Deve ser chamado uma única vez
// durante a inicialização do servidor — não é seguro chamar concorrentemente
// com escritas em andamento.
func SetLedgerWriteHook(fn func()) {
	ledgerWriteHook = fn
}

// notifyLedgerWritten dispara o hook, se existir. Não bloqueia o caminho de
// escrita: o hook (Manager.Wake) já é não bloqueante por construção.
func notifyLedgerWritten() {
	if ledgerWriteHook != nil {
		ledgerWriteHook()
	}
}
