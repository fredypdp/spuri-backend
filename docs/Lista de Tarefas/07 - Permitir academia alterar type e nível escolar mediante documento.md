---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Permitir academia alterar type e nível escolar mediante documento comprobativo (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento criando um fluxo dedicado, autenticado e auditável para a academia alterar `type` (`public`/`private`) e/ou `nivel_escolar` (`fundamental`/`medio`/`misto`), exigindo upload obrigatório de um documento comprobativo em PDF e validando o impacto da mudança sobre estudantes, cursos e `anos_academicos` já vinculados à configuração atual antes de permitir a alteração. Esta tarefa depende da tarefa 06 (`Reforçar validações na edição de dados cadastrais dos usuários`), que remove esses dois campos de `PUT /academia/dados`; implemente-a apenas depois que a tarefa 06 estiver concluída, ou, se implementadas juntas, garanta que a remoção do caminho antigo e a criação do caminho novo aconteçam na mesma entrega. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallback para o caminho sem documento.

## Contexto

`nivel_escolar` (`NivelEscolar`: `'fundamental' | 'medio' | 'misto'`) determina, entre outras coisas, se a academia pode ter `anos_academicos` diretamente no seu cadastro (fundamental/misto) e se pode criar cursos médios. `type` (`AcademiaType`: `'public' | 'private'`) representa a natureza jurídica da instituição (pública ou privada). Ambos os campos são estruturais o suficiente para exigir prova documental quando alterados depois do cadastro inicial — assim como `nif` e `alvara` já são exigidos como comprobativos obrigatórios no cadastro inicial da academia (`docs/Tarefas feitas/Adicionar nif alvara limite pdf e padronizar erros.md`).

Mudar `nivel_escolar` depois que a academia já tem estudantes, cursos e `anos_academicos` ativos é uma operação de alto impacto: uma academia que era apenas `fundamental` e passa a `misto` precisa manter os dados fundamentais intactos e só então habilitar médio; uma academia que era `medio` e passa a `misto` precisa validar que os `anos_academicos` do fundamental a serem habilitados não colidem com anos médios já existentes em cursos ativos. A tarefa 06 já remove a possibilidade de fazer essa mudança pelo caminho genérico e sem validação de `PUT /academia/dados`; esta tarefa cria o caminho correto, seguro e auditável para substituí-lo.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Novo endpoint | Fluxo dedicado em `multipart/form-data`, separado de `PUT /academia/dados` | Mudança de `type`/`nivel_escolar` exige documento comprobativo |
| Documento comprobativo | PDF obrigatório, mesmas regras de tamanho/assinatura já usadas para `alvara` | Máximo 10MB, `Content-Type: application/pdf`, assinatura `%PDF` |
| Armazenamento | `{codigo_academia}/Documentação formal/` | Mesmo padrão de diretório já usado para `alvara` |
| Validação de impacto | Bloquear mudança que deixaria dados ativos incompatíveis | Reaproveita a lógica de dependência já usada em `/academia/anos-academicos` |
| Auditoria | Novo evento no ledger da academia | Rastreabilidade completa de quando e por que a mudança ocorreu |
| Concorrência | Usar guarda transacional `unique_operation_guards` antes do upload | Duas alterações estruturais concorrentes da mesma academia não podem ser aceitas antes da projeção/evento ficar consultável |

---

# 1. Criar endpoint dedicado para alteração de `type`/`nivel_escolar`

## Objetivo

Permitir que a academia autenticada altere `type` e/ou `nivel_escolar`, exigindo documento comprobativo e validando o impacto sobre dados já existentes antes de aplicar a mudança.

## Regra de negócio

Criar `POST /academia/tipo-nivel-escolar`, protegido por autenticação de academia ativa, aceitando `multipart/form-data`:

| Campo | Obrigatório | Descrição |
| --- | --- | --- |
| `type` | Não | Novo valor de `AcademiaType` (`public`/`private`), se estiver sendo alterado |
| `nivel_escolar` | Condicional | Novo valor de `NivelEscolar` (`fundamental`/`medio`/`misto`), aceito apenas quando `nivel="escola"`; obrigatório se a academia quiser alterar este campo |
| `documento_comprovativo` | Sim, quando `type` ou `nivel_escolar` forem enviados | PDF comprobativo da mudança (ex.: alvará atualizado, certidão de alteração de natureza jurídica, licença de nível de ensino) |
| `motivo` | Sim | Justificativa textual não vazia da mudança |

Pelo menos um dos campos `type` ou `nivel_escolar` deve ser informado; caso contrário, retornar `400`.

## Escopo obrigatório

### 1.1 Validação do documento

Reaproveitar exatamente as mesmas regras já usadas para `alvara` no cadastro de academia: `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e limite máximo de 10MB.

### 1.2 Armazenamento

Salvar o documento em `{codigo_academia}/Documentação formal/`, com nome que preserve histórico (ex.: `comprovativo_alteracao_tipo_nivel_{timestamp}.pdf`), sem sobrescrever o `alvara` original nem comprovativos de mudanças anteriores.

### 1.3 Validação de impacto para `nivel_escolar`

Antes de aplicar a mudança de `nivel_escolar`, validar:

1. **De `fundamental`/`misto` para `medio`**: bloquear se existir qualquer estudante com `status_escolar_fundamental = "em_andamento"` ou qualquer `ano_academico` fundamental ativo na academia, retornando `409 Conflict` com detalhe de quantos estudantes/anos impedem a mudança — mesma filosofia de bloqueio já usada em `DELETE /academia/anos-academicos`.
2. **De `medio` para `fundamental`**: bloquear se existir qualquer curso médio ativo com estudantes vinculados.
3. **Para `misto`** (a partir de `fundamental` ou de `medio`): permitir sempre, pois `misto` é uma ampliação que preserva os dados do nível já existente; validar apenas que não há conflito de numeração entre anos fundamentais e anos médios (reaproveitando a mesma distinção por `type`/`nivel` já usada na academia mista, conforme `docs/Tarefas feitas/Permitir às academias adicionar ou remover anos acadêmicos com validações avançadas.md`).
4. **De `misto` para `fundamental` ou `medio`**: bloquear se existir qualquer dado ativo (estudante, curso, `ano_academico`) do nível que deixaria de ser suportado.

### 1.4 Validação de impacto para `type`

Alterar `type` (`public`/`private`) tem impacto estrutural bem menor. Ainda assim, exigir documento comprobativo e registrar evento auditável, sem validação adicional de dependências além da já feita para os campos comuns de `PUT /academia/dados`.

### 1.5 Guarda de unicidade em progresso

Antes de salvar o PDF temporário e antes de gravar `AcademiaTipoNivelEscolarAlterado`, reservar uma chave na guarda transacional compartilhada `unique_operation_guards`. Não usar `sync.Mutex`, mapas em memória, sleeps ou validação dependente de uma única instância da API.

Chave canônica obrigatória:

```text
academia_tipo_nivel_escolar:alteracao_em_andamento:{codigo_academia}
```

Enquanto existir outra alteração estrutural da mesma academia reservada/ativa, a rota deve retornar `409 Conflict` com mensagem clara de operação em andamento. A reserva deve ser liberada se qualquer etapa falhar antes da gravação do evento; após o evento ser persistido, a reserva deve ser consumida/finalizada de forma que uma nova alteração futura só seja bloqueada pela regra de negócio, não por resíduo da requisição anterior. Logs devem registrar `scope` e hash/chave mascarada, sem expor NIF, email, telefone ou outro dado sensível.

### 1.6 Evento auditável

Emitir um evento `AcademiaTipoNivelEscolarAlterado` no ledger da academia, contendo, no mínimo:

```json
{
  "codigo_academia": "string",
  "type_anterior": "private",
  "type_novo": "public",
  "nivel_escolar_anterior": "fundamental",
  "nivel_escolar_novo": "misto",
  "motivo": "string",
  "documento_comprovativo_path": "string",
  "alterado_por": "uuid-do-usuario-academia",
  "alterado_em": "RFC3339"
}
```

Apenas os campos efetivamente alterados precisam ser diferentes de nulo/vazio entre "anterior" e "novo"; se só `type` for alterado, os campos de `nivel_escolar` no evento devem refletir o mesmo valor antes/depois (sem mudança).

### 1.7 Resposta

```json
{
  "message": "type e/ou nivel_escolar atualizados com sucesso",
  "type": "public",
  "nivel_escolar": "misto"
}
```

### 1.8 Testes obrigatórios

1. alteração de `type` isolada, com documento válido: sucesso, evento auditável gravado;
2. alteração de `nivel_escolar` de `fundamental` para `misto`, sem estudantes conflitantes: sucesso;
3. alteração de `nivel_escolar` de `fundamental`/`misto` para `medio` com estudante fundamental `em_andamento`: rejeitado com `409`;
4. alteração de `nivel_escolar` de `medio` para `fundamental` com curso médio ativo com estudantes: rejeitado com `409`;
5. alteração sem `documento_comprovativo`: rejeitado com `400`;
6. alteração sem `motivo` ou com `motivo` vazio: rejeitado com `400`;
7. documento não-PDF ou acima de 10MB: rejeitado com `400`;
8. payload sem `type` nem `nivel_escolar`: rejeitado com `400`;
9. `nivel_escolar` enviado para academia com `nivel="superior"`: rejeitado com `400`;
10. duas requisições simultâneas para `POST /academia/tipo-nivel-escolar` da mesma academia: no máximo uma grava evento e a outra retorna `409`;
11. falha simulada antes da gravação do evento libera a guarda e permite nova tentativa válida.

---

# 2. Consultas e auditoria

## Objetivo

Permitir que a própria academia e administradores consultem o histórico de alterações de `type`/`nivel_escolar`.

## Escopo obrigatório

### 2.1 Exposição no histórico de eventos

O evento `AcademiaTipoNivelEscolarAlterado` deve aparecer nas consultas já existentes de auditoria da academia (mesmo padrão de `GET /eventos-estudante/:codigo`, mas para o aggregate de academia, se já existir consulta equivalente; caso não exista, avaliar se deve ser criada como parte desta tarefa ou registrada como item futuro).

### 2.2 Documento comprobatório na consulta autenticada

Assim como `documentos.alvara.download_url` já é exposto para usuários autenticados em `GET /academias` e `GET /consultar-academia/:codigo`, o(s) comprovativo(s) de alteração de `type`/`nivel_escolar` devem ficar disponíveis para download autenticado, seguindo o mesmo padrão de rota (`/documentos/academias/{codigo_academia}/...`).

---

# 3. Atualização obrigatória da documentação

Atualizar `Documentação.md`, seção 6 (Academias), incluindo:

- o novo endpoint `POST /academia/tipo-nivel-escolar`, contrato completo, exemplos e erros;
- a remoção de `type`/`nivel_escolar` de `PUT /academia/dados` (coordenado com a tarefa 06);
- a lista de validações de impacto por transição de `nivel_escolar`;
- o novo evento `AcademiaTipoNivelEscolarAlterado`;
- o possível `409 Conflict` por `unique_operation_in_progress`/operação estrutural em andamento para a mesma academia.

---

# Fora de escopo

- Permitir mudança do campo `nivel` (`AcademiaNivel`: `'escola'|'superior'`) — trocar entre escola e ensino superior não está no escopo desta tarefa, pois envolve modelos de dados incompatíveis entre si (cursos, matérias, avaliação final e progressão são completamente diferentes entre os dois `nivel`).
- Migrar dados de estudantes/cursos entre `nivel_escolar` diferentes (a mudança só é permitida quando não há dado ativo incompatível; não há migração automática de dados incompatíveis).
- Reabrir a alteração de `type`/`nivel_escolar` por `PUT /academia/dados`.
- Criar fluxo de aprovação por Admin para esta mudança (diferente da tarefa 08, que trata de aprovação de documentos; esta tarefa assume que a própria academia é encarregado pela mudança, mediante documento comprobativo, sem aprovação externa adicional, salvo decisão explícita em contrário).

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `POST /academia/tipo-nivel-escolar` existir, exigindo documento comprobativo e `motivo`;
2. as validações de impacto por transição de `nivel_escolar` da seção 1.3 estarem implementadas e testadas;
3. o documento comprobativo estar armazenado em `{codigo_academia}/Documentação formal/` e acessível por download autenticado;
4. o evento `AcademiaTipoNivelEscolarAlterado` estar gravado no ledger e auditável;
5. `PUT /academia/dados` não aceitar mais `type` nem `nivel_escolar` (coordenado com a tarefa 06);
6. `Documentação.md` estar atualizada com o novo endpoint e evento;
7. testes automatizados cobrirem os cenários da seção 1.8, incluindo concorrência real com goroutines/requisições simultâneas;
8. o PR explicar claramente a relação desta tarefa com a tarefa 06 e listar a chave de guarda `academia_tipo_nivel_escolar:alteracao_em_andamento:{codigo_academia}`.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Permitir academia alterar type e nível escolar mediante documento comprobativo (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
