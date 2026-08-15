# Tarefa para o Codex — Repositório `spuri-backend` (backend)

**Repositório:** https://github.com/fredypdp/spuri-backend
**Branch base:** main
**Execução:** Não é necessário planejar nada. Siga os passos exatamente como descritos abaixo, na ordem, e faça as substituições literais indicadas. Se algum trecho não bater 100% com o que está descrito aqui, PARE e reporte a diferença em vez de improvisar.

---

## Contexto do problema

O nome de província `'CUANDO CUBANGO'` (com código `CND`) está desatualizado — desde a divisão administrativa de 2024, a antiga província "Cuando Cubango" virou duas: **Cuando** (`CND`) e **Cubango** (`CBG`). O nome correto de exibição para o código `CND` é apenas **"CUANDO"**.

**Importante — diagnóstico já feito, não repita a investigação:** neste backend, esse texto errado **só existe em documentação**, não em código funcional. Já foram checados os dois pontos do código Go que lidam com província:

1. `internal/utils/validation.go`, função `ValidateProvincia` — só valida códigos (`CND`, `CBG`, etc.), nunca nomes por extenso. **Não tem o bug.**
2. `internal/handlers/academia_handlers.go`, função `validarProvincia` — já mapeia `"cuando"` → `"CND"` corretamente. Ela também aceita `"cuando cubango"` como alias de entrada legada apontando para `"CBG"` — isso é compatibilidade com dados/textos antigos, **não é o mesmo bug** e **não deve ser alterado** nesta tarefa.

O único lugar a corrigir é a documentação, onde a mesma tabela de "21 províncias" do frontend foi copiada e ficou com o nome errado.

**Objetivo da tarefa:** corrigir apenas a documentação, sem tocar em nenhum arquivo `.go`.

---

## Passo 1 — Editar `Documentação da API.md`

Arquivo: `Documentação da API.md` (raiz do repositório)

Localize, dentro da tabela de "21 províncias" (por volta da linha 1093), esta linha exata:

```
	  { nome: 'CUANDO CUBANGO', codigo: 'CND' },
```

(repare que a linha começa com um caractere de tab seguido de dois espaços, igual às linhas vizinhas da lista, por exemplo `	  { nome: 'CABINDA', codigo: 'CAB' },`)

Substitua por:

```
	  { nome: 'CUANDO', codigo: 'CND' },
```

Não altere nenhuma outra linha da tabela, incluindo a linha `{ nome: 'CUBANGO', codigo: 'CBG' },`, que já está correta.

---

## Passo 2 — Buscar por qualquer outra ocorrência esquecida em documentação

Rode, na raiz do repositório:

```bash
grep -rniI "CUANDO CUBANGO" . --include="*.md"
```

O resultado esperado é **vazio** (nenhuma ocorrência) após o Passo 1. Se aparecer outro arquivo `.md` com o mesmo texto, corrija-o do mesmo jeito.

---

## Passo 3 — Confirmar que o alias legado no código Go permanece intacto

Rode, na raiz do repositório:

```bash
grep -n "cuando" internal/handlers/academia_handlers.go
```

O resultado esperado deve continuar mostrando a linha (não mexer nela):

```go
"cuando cubango": "CBG", "cubango": "CBG", "cuando": "CND",
```

Essa linha **não faz parte desta tarefa** e deve permanecer exatamente como está — é tratamento de compatibilidade com entradas de texto livre antigas, uma decisão de produto que não deve ser alterada sem confirmação explícita separada.

---

## Passo 4 — Verificação de build

Rode, na raiz do repositório:

```bash
go build ./...
```

Como esta tarefa só altera um arquivo `.md`, o build não deve apresentar nenhuma diferença de comportamento — rode apenas para confirmar que nada foi quebrado acidentalmente.

---

## O que NÃO fazer (fora de escopo)

- **Não editar nenhum arquivo `.go`.** Em especial, não alterar `internal/utils/validation.go` nem `internal/handlers/academia_handlers.go`.
- Não remover, adicionar ou alterar o alias `"cuando cubango": "CBG"` em `validarProvincia`.
- Não alterar nenhum outro nome de província na tabela de documentação.
- Não alterar códigos de província em nenhum lugar.

---

## Resumo das mudanças (checklist final)

- [ ] `Documentação da API.md` — linha `{ nome: 'CUANDO CUBANGO', codigo: 'CND' }` → `{ nome: 'CUANDO', codigo: 'CND' }`
- [ ] `grep -rniI "CUANDO CUBANGO" . --include="*.md"` retorna vazio
- [ ] Nenhum arquivo `.go` foi modificado
- [ ] `go build ./...` passa sem erros
- [ ] Commit com mensagem sugerida: `docs: corrige nome da província CND de "CUANDO CUBANGO" para "CUANDO" na documentação`
