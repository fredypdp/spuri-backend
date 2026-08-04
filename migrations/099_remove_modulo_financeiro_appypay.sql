-- ============================================================================
-- MIGRATION 099 — Remover projeções do módulo financeiro/AppyPay (rollback)
-- ============================================================================
--
-- CONTEXTO:
--   O módulo financeiro/pagamento (AppyPay), introduzido pelas migrations 097
--   e 098, foi removido por completo do código da aplicação após três rondas
--   de auditoria de segurança identificarem falhas críticas e de alta
--   gravidade ainda não resolvidas de forma consistente (ver
--   docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay 1.md
--   e os dois relatórios de verificação subsequentes). A tarefa original
--   (docs/Lista de Tarefas/15 - Modulo base de gestao financeira com AppyPay.md)
--   volta ao estado pendente para uma reimplementação futura mais robusta.
--
--   Nenhum provider HTTP real da AppyPay chegou a ser implementado (apenas
--   FakeProvider); não existem cobranças, credenciais ou webhooks reais a
--   reconciliar externamente antes desta remoção.
--
--   As migrations 097 e 098 NÃO são apagadas — seguem o mesmo padrão já usado
--   na migration 046 (que remove projection_aprovacao_ano/projection_reprovacoes
--   sem apagar as migrations 003/009 que as criaram): o histórico de schema é
--   append-only, apenas esta migration neutraliza as tabelas.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Remove as tabelas de projeção/armazenamento operacional do módulo
--      financeiro (dados, se existirem em algum ambiente, são perdidos — não
--      há reconciliação pendente conhecida).
--   2. Remove o checkpoint da projeção "financeiro".
-- ============================================================================

BEGIN;

DROP TABLE IF EXISTS financeiro_segredos_appypay;
DROP TABLE IF EXISTS financeiro_webhooks_recebidos;
DROP TABLE IF EXISTS financeiro_cobrancas;
DROP TABLE IF EXISTS financeiro_credenciais_appypay;
DROP TABLE IF EXISTS financeiro_modalidade_pagamento;

DELETE FROM projection_checkpoints WHERE projection_name = 'financeiro';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 099 — módulo financeiro/AppyPay removido (rollback completo)';
    RAISE NOTICE '   Tabelas financeiro_* removidas. Migrations 097/098 mantidas como histórico.';
END $$;
