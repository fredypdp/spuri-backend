---
criado: 2026-07-07 00:00
origem: recomendações de melhoria recebidas pelo usuário
status: pronto_para_implementacao
---

# Avaliar e implementar recomendações de melhoria 10

## Prompt recomendado para executar a atualização

Revise as recomendações de melhoria 10.1 a 10.5 descritas neste documento, confirme no código quais pontos já estão implementados e implemente apenas os itens ainda necessários. Preserve a semântica de Event Sourcing/CQRS: mutações continuam no ledger, projeções continuam rebuildáveis e novos logs operacionais que não sejam eventos de domínio devem ficar em tabelas próprias. Ao final, atualize testes e documentação técnica afetada.

## Contexto

As recomendações recebidas misturam três tipos de situação:

1. regras de negócio que já foram implementadas e precisam apenas ser reconhecidas como decisão vigente;
2. regras parcialmente atendidas, mas que merecem teste/regressão ou ajuste de contrato;
3. melhorias de segurança/operabilidade ainda necessárias.

Este documento explica cada recomendação, classifica se ela é boa/necessária para o estado atual do backend e, quando aplicável, descreve uma tarefa de implementação no mesmo padrão das demais tarefas do projeto.

## Resumo executivo

| Item | Recomendação | Avaliação | Tarefa necessária? |
| --- | --- | --- | --- |
| 10.1 | Arquivamento de estudante via desvincular/revincular, preservando histórico | Boa e necessária, mas já implementada no domínio atual | Não implementar de novo; manter testes de regressão |
| 10.2 | Validar data da falta dentro do ano letivo ativo | Boa e necessária; o handler atual já valida registro e atualização de falta | Não implementar regra principal; reforçar testes se houver lacuna |
| 10.3 | Permitir re-registro de nota deletada com a mesma chave | Boa se o produto aceitar correção de erro operacional; exige decisão explícita | Sim, se a decisão de produto for permitir re-registro |
| 10.4 | Auditoria de acessos de leitura | Boa e necessária para dados sensíveis | Sim |
| 10.5 | Rate limiting real | Boa e necessária, especialmente para autenticação, email e bootstrap | Sim |

---

# 10.1 Arquivamento de estudante

## Explicação da recomendação

A recomendação define que `arquivado` é um status operacional do estudante quando ele saiu da academia, mas o histórico precisa continuar preservado. A academia não deveria alterar esse status diretamente. Ela deve executar um comando de domínio:

- `POST /academia/estudante/:codigo/desvincular`, que registra `EstudanteDesvinculadoDaAcademia` e coloca o estudante em `status = arquivado`;
- `POST /academia/estudante/:codigo/revincular`, que registra `EstudanteReintegrado` e retorna o estudante para `status = ativo`.

A parte mais importante é a preservação de progressão e histórico. Desvinculação, interrupção, trancamento e reativação não devem apagar notas, faltas, financeiro, auditoria, eventos ou posição acadêmica. Quando o estudante volta ao mesmo curso, a posição anterior deve ser mantida. Quando muda de curso médio ou superior, o novo vínculo começa no início do novo curso, mantendo o histórico anterior consultável.

## Avaliação

Esta recomendação é boa e necessária porque evita tratar saída temporária ou administrativa como recriação do estudante. Ela também combina com o modelo de Event Sourcing: o acontecimento real fica no ledger e as projeções derivam o estado atual.

No estado atual do backend, a regra principal já está implementada:

- existe evento de desvinculação `EstudanteDesvinculadoDaAcademia`;
- existe evento de reintegração `EstudanteReintegrado`;
- a desvinculação aplica `status = arquivado`;
- a reintegração aplica `status = ativo`;
- a reintegração preserva curso/ano/semestre quando o curso informado é o mesmo ou quando nenhum novo curso é informado;
- a reintegração reinicia para `1_ano_medio` quando há mudança real de curso médio;
- a reintegração reinicia para `1_semestre` e `1_ano_superior` quando há mudança real de curso superior;
- há testes de histórico de vínculo cobrindo os cenários principais.

## Decisão

Não criar nova implementação para 10.1. A recomendação deve ser considerada **aceita e já atendida**.

## Cuidados futuros

1. Não adicionar endpoint que altere `status = arquivado` diretamente sem evento de domínio.
2. Não permitir que payload de revinculação receba ano/semestre arbitrário do cliente.
3. Não limpar progressão acadêmica em eventos de status/vínculo.
4. Manter testes de replay para garantir que reconstrução de projeções preserve o mesmo estado final.

---

# 10.2 Validação de data de falta

## Explicação da recomendação

O problema descrito é que uma falta poderia ser registrada fora do período do ano letivo ativo da academia. Exemplo: a academia está no ano letivo `2025_2026`, mas alguém registra falta em data pertencente a `2024_2025` ou `2026_2027`.

A recomendação é validar no handler, antes de salvar o evento, se a data da falta pertence ao período fixo do ano letivo ativo. A validação deve considerar o tipo letivo da matéria:

- escolar: período fixo escolar;
- superior: período fixo superior.

## Avaliação

A recomendação é boa e necessária porque faltas fora do calendário ativo distorcem frequência, relatórios e avaliação final.

No estado atual do backend, a regra principal já aparece implementada no fluxo de registro e atualização de falta:

- `RegistrarFaltas` resolve o ano letivo da academia, infere o tipo letivo pela matéria e chama `validarDataNoPeriodoLetivo` antes de registrar o evento;
- `AtualizarFalta` recalcula a matéria/data final e também chama `validarDataNoPeriodoLetivo` antes de atualizar o evento;
- o helper calcula o intervalo do ano letivo e rejeita datas fora do intervalo.

## Decisão

Não criar nova implementação funcional para 10.2. A recomendação deve ser considerada **aceita e já atendida**.

## Tarefa opcional de reforço

Se a cobertura de testes não estiver explícita para faltas, criar testes de handler cobrindo:

1. `POST /academia/faltas-aluno` rejeita data anterior ao início do ano letivo ativo;
2. `POST /academia/faltas-aluno` rejeita data posterior ao fim do ano letivo ativo;
3. `PUT /academia/atualizar-falta` rejeita alteração de data que tire a falta do período ativo;
4. `PUT /academia/atualizar-falta` rejeita troca de matéria quando o tipo letivo da matéria tornar a data inválida;
5. mensagens de erro informam o ano letivo e o intervalo aceito.

---

# 10.3 Nota deletada bloqueia re-registro

## Explicação da recomendação

Hoje uma nota deletada por soft delete pode continuar bloqueando novo registro com a mesma chave lógica. A chave lógica de nota é composta por:

```text
codigo_academia + ano_lectivo + periodo + materia_disciplinar_id + tipo + categoria
```

A intenção original parece ser evitar duplicidade, double-submit e inconsistência na projeção. Porém, se a nota foi deletada por engano, a academia pode precisar registrá-la novamente com a mesma combinação.

A recomendação sugere avaliar se o bloqueio é realmente desejado. Se não for, `applyNotaDeletada` deve remover a chave do mapa `NotasRegistradasPorChave`.

## Avaliação

Esta recomendação é boa, mas depende de decisão de produto.

### Quando manter o bloqueio é melhor

Manter o bloqueio é melhor se a regra de negócio disser que uma nota excluída continua sendo evidência histórica e a correção deve ocorrer por evento de atualização/restauração, nunca por recriação. Essa abordagem reduz ambiguidades: a mesma chave só existiu uma vez e todas as correções são rastreadas sobre o mesmo registro.

### Quando permitir re-registro é melhor

Permitir re-registro é melhor se a exclusão for tratada como cancelamento operacional reversível. Neste caso, uma nota deletada por erro não deveria impedir a academia de registrar a nota correta novamente. O histórico da exclusão continua auditável pelo evento `NotaDeletada`, mas a projeção ativa passa a poder ter uma nova nota com a mesma chave lógica.

### Observação técnica importante

A mudança não deve ser feita apenas no aggregate. Também é necessário verificar a constraint/índice no banco. Se existir `UNIQUE` absoluto em `projection_notas` para a chave lógica, o banco continuará bloqueando o re-registro mesmo que `NotasRegistradasPorChave` seja limpo. O comportamento correto para permitir re-registro é ter unicidade apenas entre notas ativas, por exemplo com índice único parcial `WHERE deleted_at IS NULL`.

## Decisão recomendada

Recomenda-se **permitir re-registro após soft delete**, desde que:

1. a deleção continue obrigando `motivo`;
2. a nova nota gere um novo evento `NotasRegistradas`;
3. a nota deletada continue no histórico/projeção com `deleted_at`, `deletado_por` e `motivo_exclusao`;
4. relatórios e consultas usem apenas notas ativas por padrão;
5. reconstrução por replay produza o mesmo estado final.

## Tarefa de implementação

### Objetivo

Permitir que uma nota deletada por soft delete seja registrada novamente com a mesma chave lógica, sem perder o histórico da nota deletada e sem permitir duas notas ativas com a mesma chave.

### Escopo obrigatório

#### 1. Ajustar aggregate de estudante

No `applyNotaDeletada`, remover a chave correspondente de `NotasRegistradasPorChave`.

Como `NotaDeletadaEvent` atualmente carrega `NotaID`, mas não carrega diretamente a chave lógica completa, escolher uma destas abordagens:

1. **Preferencial**: enriquecer `NotaDeletadaEvent` com os campos da chave lógica no momento da deleção (`CodigoAcademia`, `AnoLectivo`, `Periodo`, `MateriaDisciplinarID`, `Tipo`, `Categoria`) e usar esses campos no apply para remover a chave;
2. **Alternativa**: manter um mapa auxiliar no aggregate de `nota_id -> chave` preenchido em `applyNotasRegistradas`, permitindo que `applyNotaDeletada` encontre a chave pelo `NotaID`.

A abordagem preferencial é mais explícita no evento e mais fácil de auditar, mas exige ajustar handler/projeção/testes para preencher os campos no evento de deleção.

#### 2. Ajustar unicidade no banco

Garantir que `projection_notas` não tenha `UNIQUE` absoluto bloqueando notas deletadas. A regra desejada é:

```sql
CREATE UNIQUE INDEX ... ON projection_notas (
  codigo_estudante,
  codigo_academia,
  ano_lectivo,
  periodo,
  materia_disciplinar_id,
  tipo,
  categoria
)
WHERE deleted_at IS NULL;
```

Se existir constraint antiga `uq_nota_unica`, removê-la por migration e substituí-la por índice único parcial com nome claro.

#### 3. Ajustar projeção

Confirmar que `handleNotasRegistradas` insere uma nova linha e que `handleNotaDeletada` faz soft delete apenas da linha ativa pelo `nota_id`. A projeção não deve reativar a linha antiga automaticamente.

#### 4. Adicionar testes regressivos

Criar testes cobrindo:

1. registrar nota com chave X;
2. tentar registrar nota duplicada ativa com chave X e receber erro;
3. deletar nota com motivo;
4. registrar nova nota com chave X com sucesso;
5. tentar registrar outra nota ativa com chave X e receber erro;
6. replay dos eventos resulta em uma nota deletada e uma nota ativa na projeção;
7. a auditoria da nota deletada permanece consultável.

### Fora de escopo

- Restaurar a mesma nota deletada por evento `NotaRestaurada`.
- Alterar consultas históricas para esconder a nota deletada quando o endpoint for explicitamente histórico/auditável.

---

# 10.4 Auditoria de acessos de leitura

## Explicação da recomendação

Atualmente o ledger registra eventos de domínio, ou seja, mutações relevantes. Consultas de leitura, como alguém visualizar dados de um estudante, não são eventos de domínio e não devem ser gravadas no ledger. Mesmo assim, dados de estudante são sensíveis e pode ser necessário saber quem consultou o quê, quando e por qual origem.

A recomendação é criar uma auditoria separada para leituras sensíveis, em tabela própria, sem misturar isso com o event store.

## Avaliação

Esta recomendação é boa e necessária por segurança, rastreabilidade e conformidade. Ela deve ser implementada com cuidado para não transformar cada GET comum em evento de domínio e para não degradar performance em endpoints de listagem.

## Tarefa de implementação

### Objetivo

Registrar acessos de leitura a dados sensíveis de estudante em tabela de auditoria própria, separada do ledger/event store.

### Escopo obrigatório

#### 1. Criar tabela de auditoria de leitura

Criar migration com tabela semelhante a:

```sql
CREATE TABLE audit_read_accesses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id UUID,
  actor_type VARCHAR(50) NOT NULL,
  target_type VARCHAR(100) NOT NULL,
  target_id UUID,
  target_codigo VARCHAR(100),
  action VARCHAR(100) NOT NULL,
  endpoint TEXT NOT NULL,
  method VARCHAR(10) NOT NULL,
  ip_address INET,
  user_agent TEXT,
  request_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Adicionar índices por:

- `target_type`, `target_id`, `created_at`;
- `target_codigo`, `created_at`;
- `actor_id`, `created_at`;
- `created_at`.

#### 2. Criar serviço/repositório de auditoria

Criar função pequena e reutilizável para registrar leitura sem acoplar handlers ao SQL bruto. A função deve aceitar contexto do Gin ou dados equivalentes:

- usuário autenticado;
- tipo do usuário;
- IP;
- user-agent;
- endpoint/método;
- estudante alvo;
- metadados mínimos.

Falha ao gravar auditoria de leitura não deve expor dados incorretos ao usuário. Definir política explícita:

- para endpoints extremamente sensíveis, falha de auditoria pode bloquear a resposta;
- para endpoints normais, falha de auditoria deve logar erro e continuar.

A primeira implementação pode usar política `best effort` com log, desde que isso esteja documentado.

#### 3. Instrumentar endpoints sensíveis

Registrar auditoria pelo menos em endpoints que retornam dados individuais de estudante ou histórico acadêmico/financeiro/sensível, incluindo:

- consulta de estudante por código;
- notas do estudante;
- faltas do estudante;
- avaliações finais do estudante;
- documentos/anexos do estudante, se existirem;
- qualquer endpoint de registros consolidados do estudante.

Evitar registrar uma linha por item em endpoints de listagem massiva. Para listagens, registrar uma entrada agregada com filtros utilizados.

#### 4. Garantir separação do ledger

Não criar eventos de domínio para leituras. Não publicar `EstudanteVisualizado` no event store. O ledger deve continuar representando mudanças de estado, não observações de leitura.

#### 5. Adicionar testes

Criar testes cobrindo:

1. academia consulta estudante pertencente à própria academia e gera auditoria;
2. estudante consulta os próprios dados e gera auditoria;
3. tentativa proibida não deve registrar leitura bem-sucedida; opcionalmente registrar tentativa negada em metadados/tabela separada;
4. filtros de listagem são gravados sem payload sensível excessivo;
5. falha do insert de auditoria segue a política definida.

### Critérios de aceite

- Existe migration da tabela de auditoria.
- Existem índices mínimos para consulta por estudante, ator e data.
- Endpoints sensíveis registram acessos de leitura.
- Nenhum evento de leitura é gravado no ledger.
- Testes documentam o comportamento.

---

# 10.5 Rate limiting

## Explicação da recomendação

A recomendação aponta que os middlewares de rate limit estão desativados: eles apenas chamam `c.Next()` e deixam todas as requisições passarem.

Isso é perigoso especialmente em endpoints de:

- login;
- envio/validação de email;
- bootstrap/configuração inicial;
- endpoints públicos ou de alto custo.

Rate limiting real reduz brute force, abuso de envio de email, scraping e picos acidentais.

## Avaliação

Esta recomendação é boa e necessária. O arquivo atual já importa `golang.org/x/time/rate`, mas a implementação é apenas compatibilidade de API. Portanto, deve ser implementada.

## Tarefa de implementação

### Objetivo

Ativar rate limiting real, seguro para uso inicial em instância única, com caminho claro para Redis/distribuído no futuro se houver múltiplas réplicas.

### Escopo obrigatório

#### 1. Implementar limiter em memória

Usar `golang.org/x/time/rate` com armazenamento por chave. A chave deve considerar:

- IP do cliente;
- tipo de rota;
- opcionalmente identificador do usuário quando autenticado.

Implementar limpeza periódica de chaves antigas para evitar crescimento infinito de memória.

#### 2. Preservar API pública atual

Manter funções existentes para reduzir impacto nas rotas:

- `NewRateLimiter`;
- `RateLimitMiddleware`;
- `GlobalRateLimit`;
- `LoginRateLimit`;
- `EmailRateLimit`.

Elas não devem mais retornar apenas `c.Next()`.

#### 3. Definir políticas iniciais

Valores sugeridos para começar:

- global: limite moderado por IP, suficiente para navegação normal;
- login: limite agressivo por IP e, se possível, por identificador/email informado;
- email: limite agressivo por IP e email/destinatário;
- bootstrap: limite muito restrito por IP.

Os valores finais devem ser configuráveis por ambiente, com defaults seguros.

#### 4. Resposta padronizada ao exceder limite

Quando exceder o limite, responder `HTTP 429 Too Many Requests` com JSON padronizado:

```json
{
  "error": "muitas requisições, tente novamente mais tarde"
}
```

Adicionar header `Retry-After` quando possível.

#### 5. Considerar proxy/reverse proxy

Usar `c.ClientIP()` e revisar configuração de trusted proxies do Gin/deploy. Não confiar cegamente em `X-Forwarded-For` se a aplicação não estiver configurada para aceitar apenas proxies confiáveis.

#### 6. Adicionar testes

Criar testes cobrindo:

1. requisições dentro do limite passam;
2. requisição acima do limite retorna 429;
3. buckets são separados por IP/chave;
4. login/email/bootstrap usam limites mais restritivos que o global;
5. limpeza de buckets antigos não remove limiter ativo indevidamente.

### Critérios de aceite

- Os middlewares não são mais no-op.
- Login, email e bootstrap têm limites específicos.
- Excesso retorna 429 com resposta consistente.
- Testes cobrem passagem, bloqueio e separação por chave.
- A documentação operacional informa como ajustar limites por ambiente.

---

# Ordem sugerida de execução

1. **Rate limiting** primeiro, por reduzir risco imediato de abuso em endpoints públicos.
2. **Auditoria de leituras** em seguida, por ser melhoria estrutural de segurança e conformidade.
3. **Nota deletada e re-registro** depois da decisão de produto, porque altera contrato de dados e migration de unicidade.
4. **Testes/documentação de faltas** apenas se a cobertura atual for insuficiente.
5. **Arquivamento de estudante** apenas manter como regra vigente e proteger com regressão.

# Checklist final para implementação futura

- [ ] Confirmar decisão de produto sobre re-registro de nota deletada.
- [ ] Se permitir re-registro, ajustar aggregate, migration, projeção e testes.
- [ ] Criar tabela de auditoria de leitura fora do ledger.
- [ ] Instrumentar endpoints sensíveis de leitura.
- [ ] Implementar rate limiting real.
- [ ] Adicionar testes automatizados.
- [ ] Atualizar documentação de API/operacional afetada.
