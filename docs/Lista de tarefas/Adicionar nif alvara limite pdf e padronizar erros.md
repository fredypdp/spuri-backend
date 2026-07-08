---
criado: 2026-07-08 00:00
origem: solicitação do usuário
status: pendente
---

# Adicionar NIF, alvará obrigatório, limite PDF de 10MB e padronizar erros

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que academias passem a possuir o campo `nif` como string de exatamente 10 dígitos, que o cadastro de academias exija documentação formal por meio do documento obrigatório `alvara`, que todos os uploads de PDF aceitos pelo sistema passem a respeitar limite máximo de 10MB e que todas as rotas retornem erros exclusivamente no padrão mais recente. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a código legado, aliases, wrappers de compatibilidade, fallbacks temporários ou respostas de erro no modelo legado.

## Contexto

A regra de produto evoluiu para tornar o cadastro de academias mais rigoroso e auditável, exigindo identificação fiscal e documentação formal desde o cadastro. O `nif` passa a compor os dados obrigatórios da academia e deve ser tratado como string, ainda que represente um código numérico de 10 dígitos, para preservar zeros à esquerda e evitar conversões indevidas.

Além disso, o documento formal `alvara` passa a ser obrigatório para cadastro de academias. Esse arquivo deve ser armazenado dentro do diretório da própria academia, na pasta `Documentação formal`, seguindo a estrutura `{codigo_academia}/Documentação formal/`.

Também fica estabelecido que todo PDF enviado pelo sistema deve ter limite máximo de 10MB, sem exceções por rota, entidade ou tipo documental. Por fim, as respostas de erro devem ser padronizadas em todas as rotas usando somente o padrão mais recente, removendo integralmente o formato legado ainda utilizado pelo backend.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| `nif` da academia | Novo campo obrigatório tratado como string | Aceitar apenas string com exatamente 10 dígitos |
| `alvara` | Documento formal obrigatório no cadastro de academias | Exigir upload e salvar em `{codigo_academia}/Documentação formal/` |
| Limite de PDFs | 10MB para todos os uploads de PDF | Rejeitar qualquer PDF acima de 10MB em todas as rotas |
| Erros de API | Usar somente o padrão mais recente | Remover completamente respostas no modelo legado |
| Documentação | Atualizar integralmente | Contratos, guias e exemplos devem refletir as novas regras |
| Legado | Proibido | Não manter aliases, compatibilidade temporária, fallback ou formato antigo de erro |

---

# 1. Adicionar campo `nif` na academia

## Objetivo

Garantir que toda academia possua um `nif` válido, persistido como string e validado como código de exatamente 10 dígitos.

## Regra de negócio

Ao criar ou atualizar uma academia, o backend deve:

1. aceitar `nif` como string;
2. exigir exatamente 10 caracteres numéricos (`0` a `9`);
3. preservar zeros à esquerda;
4. rejeitar valores enviados como número quando o contrato distinguir tipos;
5. rejeitar strings com espaços, pontuação, letras, menos de 10 dígitos ou mais de 10 dígitos;
6. persistir e retornar o valor exatamente como validado;
7. incluir o campo nas consultas e respostas públicas/administrativas da academia conforme o contrato vigente.

## Escopo obrigatório

### 1.1 Ajustar contratos de entrada e saída

Atualizar DTOs, schemas, validators, commands, handlers e documentação de API para incluir `nif` nos contratos de academia.

O campo deve ser documentado explicitamente como string de 10 dígitos. Não documentar o campo como número inteiro.

### 1.2 Ajustar modelo de domínio e persistência

Atualizar entidades, aggregates, migrations, repositories, projections e serializers para persistir `nif` na academia.

A persistência deve usar tipo textual compatível com 10 caracteres e constraints ou validações que impeçam estados inválidos sempre que possível.

### 1.3 Validar unicidade, se aplicável

Avaliar com o domínio se duas academias podem compartilhar o mesmo `nif`. Se o NIF for identificador fiscal único, criar constraint de unicidade e testes específicos. Caso a decisão seja não aplicar unicidade nesta tarefa, justificar claramente no PR e na documentação técnica.

### 1.4 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. criação de academia com `nif` válido de 10 dígitos;
2. preservação de zeros à esquerda;
3. rejeição de `nif` ausente quando obrigatório;
4. rejeição de `nif` com menos de 10 dígitos;
5. rejeição de `nif` com mais de 10 dígitos;
6. rejeição de `nif` com letras, espaços ou pontuação;
7. resposta de consulta de academia retornando `nif` conforme contrato;
8. migração/persistência impedindo estado inválido quando houver constraint aplicável.

---

# 2. Cobrar alvará obrigatório no cadastro de academias

## Objetivo

Exigir documentação formal para o cadastro de academias por meio do documento obrigatório `alvara`.

## Regra de negócio

Ao cadastrar uma academia, o backend deve:

1. exigir o arquivo `alvara` como documento obrigatório;
2. aceitar apenas PDF, respeitando o limite global de 10MB;
3. salvar o documento na pasta `Documentação formal` dentro do diretório da academia;
4. usar a estrutura `{codigo_academia}/Documentação formal/`;
5. associar o documento à academia de forma rastreável;
6. impedir que a academia seja criada ou ativada sem o documento obrigatório, conforme o fluxo vigente de cadastro;
7. retornar erro padronizado quando o documento estiver ausente, inválido ou acima do limite.

## Escopo obrigatório

### 2.1 Ajustar fluxo de cadastro de academia

Atualizar handlers, serviços, casos de uso e integrações de upload para receber e validar o documento `alvara` no cadastro de academia.

Se o cadastro de academia possuir etapas assíncronas ou transacionais, garantir atomicidade suficiente para não deixar academia criada sem documento obrigatório quando o upload falhar.

### 2.2 Ajustar armazenamento

Garantir que o arquivo seja salvo no diretório da academia, dentro da pasta `Documentação formal`:

```text
{codigo_academia}/Documentação formal/
```

A implementação deve respeitar o mecanismo de storage vigente do projeto e manter nomes, metadados, logs e referências consistentes com os demais documentos formais.

### 2.3 Atualizar consultas e auditoria

Garantir que o backend consiga consultar, auditar ou referenciar o `alvara` da academia conforme os padrões existentes de documentos.

Se houver endpoints administrativos de visualização, listagem ou download de documentos, incluir o `alvara` conforme o contrato de autorização vigente.

### 2.4 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. cadastro de academia com `alvara` válido;
2. rejeição de cadastro sem `alvara`;
3. rejeição de `alvara` que não seja PDF;
4. rejeição de `alvara` acima de 10MB;
5. persistência do documento em `{codigo_academia}/Documentação formal/`;
6. falha de upload não deixando cadastro inconsistente;
7. resposta de erro padronizada para ausência ou invalidade do `alvara`;
8. autorização adequada para consultar ou baixar o documento, se houver endpoints para isso.

---

# 3. Aplicar limite global de 10MB para PDFs

## Objetivo

Padronizar o tamanho máximo de todos os arquivos PDF enviados pelo sistema para 10MB.

## Regra de negócio

Todo upload de PDF deve:

1. validar tamanho máximo de 10MB;
2. aplicar a validação antes de persistir o arquivo;
3. retornar erro padronizado quando o arquivo exceder o limite;
4. usar a mesma regra em todas as rotas, independentemente do tipo documental;
5. evitar limites divergentes em handlers, middlewares, serviços, clientes de storage, documentação e testes.

## Escopo obrigatório

### 3.1 Centralizar ou padronizar a constante de limite

Auditar todas as validações de upload e substituir limites divergentes por uma regra única de 10MB para PDFs.

Preferir constante compartilhada ou helper centralizado para evitar regressões futuras.

### 3.2 Auditar todas as rotas de upload

Revisar todas as rotas que aceitam PDF, incluindo, quando existirem:

- documentos de estudantes;
- solicitações de matrícula;
- documentos de responsáveis;
- documentos formais de academias;
- importações administrativas;
- anexos usados por jobs assíncronos;
- qualquer outro fluxo que aceite `application/pdf`.

### 3.3 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. PDF com tamanho menor que 10MB é aceito;
2. PDF com exatamente 10MB é aceito, se a regra do projeto considerar limite inclusivo;
3. PDF acima de 10MB é rejeitado;
4. todas as rotas de upload aplicam o mesmo limite;
5. mensagem de erro segue o padrão mais recente;
6. documentação e OpenAPI informam 10MB como limite máximo.

---

# 4. Padronizar mensagens de erro no padrão mais recente

## Objetivo

Garantir que todas as rotas retornem erros aos usuários exclusivamente no padrão mais recente, abandonando por completo o modelo legado.

## Regra de negócio

Todas as respostas de erro devem:

1. usar o envelope/corpo definido pelo padrão mais recente do backend;
2. conter códigos, mensagens e detalhes conforme o contrato atual;
3. ser consistentes entre validação, autenticação, autorização, domínio, upload, infraestrutura e erros inesperados;
4. não retornar campos, formatos, wrappers ou mensagens do modelo legado;
5. manter status HTTP correto para cada classe de erro;
6. preservar rastreabilidade interna sem expor detalhes sensíveis ao usuário.

## Escopo obrigatório

### 4.1 Identificar padrão atual e modelo legado

Auditar handlers, middlewares, helpers, serializers e testes para identificar:

- o padrão mais recente de erro;
- todos os pontos que ainda retornam o modelo legado;
- diferenças de envelope, nomes de campos, códigos e estrutura de detalhes;
- respostas manuais escritas diretamente em handlers.

### 4.2 Remover modelo legado

Remover ou substituir completamente helpers, structs, aliases, wrappers, funções utilitárias e caminhos de resposta que emitam o formato antigo.

Não manter fallback, compatibilidade temporária, configuração por rota, header opt-in, query param, alias de campo ou conversor automático para o formato legado.

### 4.3 Padronizar todas as rotas

Atualizar todas as rotas para usar a camada padronizada de erro, incluindo:

- autenticação e autorização;
- cadastro e login;
- academias;
- estudantes;
- solicitações de matrícula;
- cursos, matérias, turmas e anos acadêmicos;
- notas, faltas, avaliações e regras finais;
- jobs assíncronos e SSE, quando retornarem erro HTTP;
- uploads e downloads;
- healthcheck e endpoints administrativos, se aplicável.

### 4.4 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. erro de validação no padrão novo;
2. erro de autenticação no padrão novo;
3. erro de autorização no padrão novo;
4. erro de domínio no padrão novo;
5. erro de upload no padrão novo;
6. erro de recurso não encontrado no padrão novo;
7. erro interno sanitizado no padrão novo;
8. ausência completa dos campos do modelo legado nas respostas;
9. snapshots ou assertions de contrato para rotas representativas;
10. documentação e OpenAPI refletindo apenas o padrão mais recente.

---

# 5. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir os novos campos, documentos obrigatórios, limite de upload e formato de erro.

## Escopo de documentação

Atualizar, quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio de academias;
- documentação de uploads e storage;
- documentação de erros da API;
- exemplos de payload;
- coleções de API;
- guias operacionais;
- documentos de tarefas anteriores usados como referência ativa.

## Regras de documentação

A documentação deve declarar explicitamente que:

- `nif` é string obrigatória com exatamente 10 dígitos;
- `alvara` é documento obrigatório para cadastro de academia;
- o `alvara` fica salvo em `{codigo_academia}/Documentação formal/`;
- todo PDF enviado ao sistema possui limite máximo de 10MB;
- todas as rotas retornam erros apenas no padrão mais recente;
- o modelo legado de erros não existe mais como contrato suportado.

---

# 6. Fora de escopo

- Tratar `nif` como número inteiro.
- Aceitar `nif` com máscara, espaços, pontuação ou letras.
- Criar cadastro de academia sem `alvara` obrigatório.
- Permitir exceções ao limite de 10MB para PDFs.
- Manter respostas de erro no modelo legado.
- Criar aliases, wrappers de compatibilidade, fallbacks temporários ou flags para reativar o formato antigo de erro.
- Alterar regras de negócio não relacionadas a academia, uploads de PDF ou padronização de erros.

---

# 7. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. academia possuir campo `nif` obrigatório tratado como string;
2. `nif` aceitar somente exatamente 10 dígitos e preservar zeros à esquerda;
3. cadastro de academia exigir `alvara` obrigatório;
4. `alvara` válido for salvo em `{codigo_academia}/Documentação formal/`;
5. falha no upload do `alvara` não deixar academia cadastrada em estado inconsistente;
6. todo upload de PDF do sistema aplicar limite máximo de 10MB;
7. PDFs acima de 10MB forem rejeitados em todas as rotas;
8. todas as respostas de erro usarem exclusivamente o padrão mais recente;
9. o modelo legado de erros tiver sido removido por completo dos handlers, helpers, testes e documentação;
10. OpenAPI/Swagger, documentação técnica e exemplos estiverem atualizados;
11. testes automatizados cobrirem `nif`, `alvara`, limite de PDFs e padronização de erros;
12. o PR explicar claramente as mudanças de contrato, persistência, storage, validação de uploads e remoção do modelo legado de erros.
