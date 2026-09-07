---
criado: 07-09-2026
origem: Fredy + Claude (localização do vazamento)
status: pendente
tipo: documentação / segurança (spuri-backend)
prioridade: ALTA — credencial real exposta em texto puro no repositório
---

# Corrigir vazamento de token na `GoSMS API - Documentação.md`

### Documento de execução para o Codex (localizado e pré-testado pelo Claude)

> **Este documento já contém tudo que é necessário.** Você (Codex) não precisa investigar nada —
> apenas aplicar as duas substituições literais da seção 2. Ambiente: `apt`/Docker/`psql` bloqueados
> no seu ambiente não importam aqui, é edição de texto puro em um único arquivo Markdown.

## 0. O que foi encontrado

Em `docs/Parceiros e integrações/GoSMS API - Documentação.md`, o token real de autenticação da GoSMS
está exposto em texto puro, sem qualquer mascaramento, em **duas** ocorrências:

- Linha 34: `Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503`
- Linha 123: `-H "Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503" \`

Confirmei, com `grep` em todo o repositório, que essas são as **únicas** duas ocorrências desse valor
em todo o working tree atual — não há uma terceira cópia escondida em outro arquivo.

O próprio documento já tem o padrão correto de como isso deveria estar escrito — na linha 379 (seção de
exemplos finais), já existe `Authorization: Token <TOKEN_REAL>`, usando um placeholder em vez do valor
real. As duas ocorrências das linhas 34 e 123 são as únicas que fogem desse padrão.

## 1. Escopo exato — só toque nisto

Arquivo: `docs/Parceiros e integrações/GoSMS API - Documentação.md`. Só as linhas 34 e 123 (os números
podem ter mudado ligeiramente se o arquivo foi editado entre a auditoria e a execução — use o texto
literal abaixo para localizar, não confie cegamente no número da linha).

## 2. Localizar/substituir

### 2.1 Primeira ocorrência

**Localizar:**
```
Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503
```

**Substituir por:**
```
Authorization: Token <TOKEN_REAL>
```

### 2.2 Segunda ocorrência

**Localizar:**
```
  -H "Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503" \
```

**Substituir por:**
```
  -H "Authorization: Token <TOKEN_REAL>" \
```

(Preserve exatamente a indentação/espaçamento original de cada linha — o trecho acima já reflete o
espaçamento visto no arquivo original; confira contra o arquivo real antes de aplicar, porque
indentação incorreta quebraria o bloco de código em que a linha está.)

## 3. Checklist de autoverificação

1. `grep -n "a9eb6ea6-5777-4848-a9ed-8cbffc74a503" "docs/Parceiros e integrações/GoSMS API - Documentação.md"` — deve retornar **vazio** depois da correção.
2. `grep -n "Token <TOKEN_REAL>" "docs/Parceiros e integrações/GoSMS API - Documentação.md"` — deve
   retornar **três** ocorrências agora (a que já existia na linha ~379 + as duas novas).
3. Releia as duas linhas alteradas no contexto (alguns blocos de código acima/abaixo) para confirmar
   que nada mais no bloco foi tocado por engano.

## 4. Fora de escopo — não faça

- Não altere mais nada nesse documento além das duas linhas indicadas.
- Não altere nenhum arquivo `.go`, `.env`, ou de configuração.
- Não tente invalidar/rotacionar o token você mesmo — isso não é possível a partir do repositório (é
  uma ação no painel da GoSMS, fora do escopo de um editor de código) e está sinalizado para Fredy
  separadamente.

## 5. Entrega

Ao terminar, reporte em texto simples:

- Confirmação de que a checklist da seção 3 foi executada e passou.
- Confirmação de que exatamente 2 linhas foram alteradas (nada a mais).

---

## Nota para Fredy (fora da tarefa do Codex — leia você mesmo)

Apagar o token do arquivo atual **não remove ele do histórico do Git** — qualquer pessoa com acesso ao
repositório (ou a um fork/clone antigo) ainda consegue recuperar esse valor navegando pelo histórico de
commits, mesmo depois deste patch aplicado. Isso só protege commits futuros a partir de agora.

Como o token já esteve exposto publicamente no histórico do repositório, a recomendação de segurança
padrão é **revogar e gerar um novo token no painel da GoSMS**, e então atualizar onde quer que esse
token esteja configurado em produção (variável de ambiente do backend, etc.) — independentemente de
reescrever ou não o histórico do Git. Reescrever o histórico (`git filter-repo`/BFG) é uma opção
adicional, mas só faz sentido depois de já ter rotacionado o token, e tem efeitos colaterais (reescreve
hashes de commit, exige force-push e recloning por qualquer colaborador) que vale avaliar com calma —
não é algo para o Codex decidir ou executar sozinho.
