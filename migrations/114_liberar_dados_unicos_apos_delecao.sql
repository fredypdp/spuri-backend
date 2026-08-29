-- MIGRATION 114 - Libera reuso de e-mail/BI/telefone após deleção de admin/estudante
--
-- Espelha o padrão já usado pela migration 111 (NIF/e-mail de Academia): troca
-- constraints/índices UNIQUE incondicionais por índices UNIQUE parciais que
-- excluem status = 'deletado'. Depois de um admin ou estudante ser deletado,
-- o dado deixa de "reservar" o valor — outro cadastro pode reutilizá-lo.
--
-- NÃO libera projection_estudantes.codigo_estudante: é usado como chave
-- natural em notas/faltas (projection_notas.codigo_estudante,
-- projection_faltas.codigo_estudante) sem FK declarada. Reutilizar o código
-- misturaria o histórico acadêmico de duas pessoas diferentes sob o mesmo
-- código — risco de integridade, não um simples "campo exclusivo".
-- codigo_academia (Academia) já é preservado pelo mesmo motivo (é alvo de FK
-- de projection_cursos/projection_materias) — nada muda aqui, é só reforço.

-- ── projection_admins.email ──────────────────────────────────────────────
ALTER TABLE projection_admins DROP CONSTRAINT IF EXISTS projection_admins_email_key;
DROP INDEX IF EXISTS projection_admins_email_key;
CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_email_ativo
    ON projection_admins (email)
    WHERE status <> 'deletado';

-- ── projection_admins: bootstrap FPP único (evita reservar role para sempre) ──
DROP INDEX IF EXISTS idx_bootstrap_fpp_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bootstrap_fpp_unique
    ON projection_admins (role)
    WHERE created_by IS NULL AND status <> 'deletado';

-- ── projection_admins.telefone (verificado) ──────────────────────────────
DROP INDEX IF EXISTS idx_telefone_verificado_admin;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_admin
    ON projection_admins (telefone)
    WHERE telefone_verificado = true AND telefone IS NOT NULL AND status <> 'deletado';

-- ── projection_estudantes.bilhete_identidade (normalizado) ──────────────
DROP INDEX IF EXISTS uq_estudante_bilhete_identidade_normalizado;
CREATE UNIQUE INDEX IF NOT EXISTS uq_estudante_bilhete_identidade_normalizado
    ON projection_estudantes (lower(btrim(bilhete_identidade::text)))
    WHERE bilhete_identidade IS NOT NULL
      AND btrim(bilhete_identidade::text) <> ''
      AND status <> 'deletado';

-- ── projection_estudantes.telefone (verificado, do próprio estudante) ───
DROP INDEX IF EXISTS idx_telefone_verificado_estudante;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_estudante
    ON projection_estudantes (telefone)
    WHERE telefone_verificado = true AND telefone IS NOT NULL AND status <> 'deletado';

-- ── projection_estudantes.telefone_encarregado (verificado) ─────────────
DROP INDEX IF EXISTS idx_telefone_resp_verificado_estudante;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_resp_verificado_estudante
    ON projection_estudantes (telefone_encarregado)
    WHERE telefone_encarregado_verificado = true
      AND telefone_encarregado IS NOT NULL
      AND status <> 'deletado';

-- ── projection_estudantes.telefone_encarregado (incondicional, legado) ──
DROP INDEX IF EXISTS idx_estudante_telefone_encarregado_unico;
CREATE UNIQUE INDEX IF NOT EXISTS idx_estudante_telefone_encarregado_unico
    ON projection_estudantes (telefone_encarregado)
    WHERE telefone_encarregado IS NOT NULL AND status <> 'deletado';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 114 - email/BI/telefone liberados para reuso após deleção'; END $$;
