---
criado: 2026-07-13 00:00
origem: solicitação do usuário
status: feito
---

# Atualizar cadastro em massa de estudantes com suporte a arquivos (feito)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que o cadastro em massa/lote de estudantes suporte requisições com arquivos e requisições apenas JSON, usando o campo `com_arquivo` para definir o modo de entrada. Quando houver arquivos, a rota deve validar e armazenar documentos com as mesmas regras das rotas singulares de cadastro de estudante e solicitação de matrícula. Quando não houver arquivos, deve aceitar e validar somente campos textuais seguindo obrigatoriamente as validações e regras específicas de JSON da rota singular `POST /academia/estudante/register`, mantendo o estudante em um status próprio de pendência documental. Também deve ser criada uma rota específica para carregar posteriormente os documentos desses estudantes, cobrando Bilhetes de Identidade conforme os dados textuais já informados e atualizando toda a documentação técnica, OpenAPI/Swagger e exemplos afetados. Não criar suporte a código legado, aliases, wrappers de compatibilidade, fallbacks temporários ou validações divergentes entre cadastro singular e cadastro em massa.

## Contexto

O fluxo atual de cadastro em massa/lote de estudantes precisa evoluir para atender dois cenários operacionais distintos da academia:

1. importações completas, em que cada estudante é enviado junto com os seus documentos; e
2. importações textuais, em que a academia registra primeiro os dados do estudante e carrega os documentos em uma etapa posterior.

A rota de lote não deve mais presumir que sempre receberá apenas uma requisição JSON. Ela deve aceitar uploads de arquivos, identificar corretamente a qual estudante cada arquivo pertence e reaproveitar as validações documentais já existentes nos fluxos singulares, evitando regras paralelas ou inconsistências entre cadastro individual, solicitação de matrícula e cadastro em massa.

Quando a importação for feita sem arquivos, o estudante não pode ficar ativo imediatamente. Ele deve ser criado em um status específico que represente pendência de arquivamento/carregamento de documentos, mantendo o cadastro rastreável e impedindo ativação indevida antes da conclusão documental.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Entrada da rota de lote | Suportar JSON e multipart/form-data | Cadastro em massa aceitar lotes com ou sem arquivos |
| `com_arquivo` | Novo campo booleano obrigatório | Definir explicitamente se a requisição traz documentos |
| Identificação de arquivos | Cada arquivo deve ser associado ao estudante correto | Evitar troca, perda ou associação ambígua de documentos |
| Validação com arquivos | Reusar regras das rotas singulares | Mesmas validações do cadastro singular e solicitação de matrícula |
| Validação sem arquivos | Validar apenas campos textuais | Seguir as regras JSON de `POST /academia/estudante/register` |
| Status sem documentos | Novo status de pendência documental | Estudante não fica ativo antes do envio dos documentos |
| Upload posterior | Nova rota específica por estudante pendente | Permitir completar documentos com as mesmas regras do cadastro singular |
| BI obrigatório posterior | Cobrar BI conforme campos textuais preenchidos | Exigir somente documentos de BI correspondentes aos dados informados |
| Documentação | Atualizar integralmente | Contratos, exemplos e OpenAPI refletirem os dois modos |

---

# 1. Atualizar a rota de cadastro em massa/lote para aceitar arquivos

## Objetivo

Permitir que a rota de cadastro em massa/lote de estudantes aceite uploads de arquivos além de requisições JSON, mantendo compatibilidade contratual explícita por meio do campo `com_arquivo`.

## Regra de negócio

A rota de cadastro em massa/lote deve:

1. aceitar requisições `multipart/form-data` quando `com_arquivo` for `true`;
2. aceitar requisições JSON quando `com_arquivo` for `false`;
3. exigir o campo `com_arquivo` como booleano real (`true` ou `false`), sem aceitar strings como `"true"`/`"false"` quando o contrato JSON distinguir tipos;
4. rejeitar requisições com arquivos quando `com_arquivo` for `false`;
5. rejeitar requisições sem arquivos obrigatórios quando `com_arquivo` for `true`;
6. identificar corretamente a qual estudante pertence cada arquivo enviado;
7. impedir associação ambígua, duplicada ou órfã de arquivos;
8. manter atomicidade suficiente para não criar estudantes ou documentos inconsistentes em caso de falha parcial;
9. retornar erros no padrão vigente do backend.

## Escopo obrigatório

### 1.1 Ajustar contrato da rota de lote

Atualizar DTOs, schemas, validators, parsers, controllers, handlers, casos de uso e documentação de API para representar os dois modos de entrada:

- modo com arquivos: `multipart/form-data`, `com_arquivo: true` e lista de estudantes com referências inequívocas aos seus arquivos;
- modo sem arquivos: `application/json`, `com_arquivo: false` e lista de estudantes contendo apenas campos textuais.

A implementação deve definir e documentar uma estratégia única para correlacionar arquivos e estudantes, por exemplo por `codigo_temporario`, índice do item no lote, chave de documento ou outro identificador estável adotado pelo projeto.

### 1.2 Identificar corretamente o dono de cada arquivo

Cada arquivo enviado no lote deve ser associado a exatamente um estudante. O backend deve validar:

1. arquivo referenciado por estudante inexistente no payload;
2. estudante referenciando arquivo ausente;
3. arquivo duplicado para o mesmo tipo documental quando a regra não permitir duplicidade;
4. arquivo órfão sem estudante correspondente;
5. colisão de identificadores entre estudantes do mesmo lote;
6. envio de documento de um estudante em campo pertencente a outro estudante.

### 1.3 Garantir consistência transacional

Se qualquer estudante, campo textual, arquivo, validação documental ou persistência falhar, o lote deve seguir a política transacional definida para o endpoint. Caso o padrão do projeto seja falhar o lote inteiro, nenhuma entidade ou arquivo parcial deve permanecer persistido. Caso exista processamento parcial documentado, o resultado deve indicar claramente os itens aceitos e rejeitados, sem deixar arquivos sem vínculo.

### 1.4 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. lote JSON sem arquivos com `com_arquivo: false`;
2. lote multipart com arquivos e `com_arquivo: true`;
3. rejeição de arquivos quando `com_arquivo: false`;
4. rejeição de ausência de arquivos obrigatórios quando `com_arquivo: true`;
5. rejeição de `com_arquivo` ausente;
6. rejeição de `com_arquivo` com tipo inválido;
7. associação correta de cada arquivo ao respectivo estudante;
8. rejeição de arquivos órfãos;
9. rejeição de referências duplicadas ou ambíguas;
10. rollback ou relatório parcial consistente em caso de falha no meio do lote.

---

# 2. Aplicar validações singulares quando o lote vier com arquivos

## Objetivo

Garantir que o cadastro em massa com documentos use exatamente as mesmas validações de documentos, campos e regras de negócio das rotas singulares de cadastro de estudante e solicitação de matrícula.

## Regra de negócio

Quando `com_arquivo` for `true`, a rota de lote deve:

1. validar campos textuais conforme o cadastro singular de estudante;
2. validar documentos conforme a rota `POST /academia/estudante/register`;
3. validar documentos também conforme as regras aplicáveis à solicitação de matrícula, quando o mesmo tipo documental existir nos dois fluxos;
4. aceitar somente tipos, formatos, tamanhos, nomes de campo e combinações documentais permitidos nos fluxos singulares;
5. rejeitar qualquer exceção criada apenas para o lote;
6. armazenar documentos com o mesmo padrão de diretórios, metadados, auditoria e vínculos das rotas singulares;
7. retornar erros padronizados e equivalentes aos fluxos singulares para as mesmas violações.

## Escopo obrigatório

### 2.1 Reusar validações existentes

A implementação deve reaproveitar validadores, helpers, serviços, policies ou casos de uso já utilizados nas rotas singulares, evitando cópia divergente de regras.

Se houver diferenças históricas entre cadastro singular de estudante e solicitação de matrícula, a tarefa deve primeiro mapear essas diferenças e consolidar o comportamento esperado para o lote com arquivos, priorizando a regra mais restritiva quando necessário para manter segurança documental.

### 2.2 Validar documentos obrigatórios e condicionais

O lote com arquivos deve respeitar as mesmas obrigatoriedades e condicionalidades já existentes, incluindo, quando aplicável:

- documento de identificação do estudante;
- Bilhete de Identidade do estudante;
- Bilhete de Identidade do responsável;
- declaração acadêmica;
- documentos exigidos por nível, curso, idade, modalidade, responsável ou tipo de matrícula;
- limites de tamanho e tipos MIME permitidos;
- nomes de arquivo, extensão e conteúdo real quando o projeto já validar esses aspectos.

### 2.3 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. lote com arquivos válidos seguindo o cadastro singular;
2. lote com documentos válidos seguindo solicitação de matrícula;
3. rejeição dos mesmos documentos inválidos rejeitados no cadastro singular;
4. rejeição dos mesmos documentos inválidos rejeitados na solicitação de matrícula;
5. armazenamento no mesmo padrão dos fluxos singulares;
6. mensagens de erro equivalentes às rotas singulares;
7. ausência de regras documentais exclusivas e divergentes para o lote.

---

# 3. Validar lote sem arquivos apenas com campos textuais

## Objetivo

Permitir que a academia cadastre estudantes em massa apenas com dados textuais, sem exigir documentos nessa primeira etapa, seguindo obrigatoriamente as validações específicas de JSON da rota singular `POST /academia/estudante/register`.

## Regra de negócio

Quando `com_arquivo` for `false`, a rota de lote deve:

1. aceitar somente payload JSON com campos textuais;
2. rejeitar qualquer arquivo enviado na requisição;
3. aplicar as validações e regras específicas de campos JSON da rota `POST /academia/estudante/register`;
4. não executar validações de presença de arquivos nessa etapa;
5. validar obrigatoriedade, formato, domínio, relacionamento e consistência dos campos textuais;
6. persistir os dados necessários para cobrar os documentos corretos posteriormente;
7. criar o estudante em status de pendência documental, não em status ativo;
8. retornar resultado por estudante conforme o padrão do endpoint de lote.

## Escopo obrigatório

### 3.1 Reusar regras JSON do cadastro singular

O lote sem arquivos deve seguir a mesma regra de validação JSON do cadastro singular `POST /academia/estudante/register`, incluindo validações de:

- dados pessoais do estudante;
- dados acadêmicos;
- dados de curso, turma, classe, ano acadêmico, período ou modalidade quando aplicável;
- dados de encarregado/responsável;
- Bilhete de Identidade informado como campo textual;
- datas, telefones, e-mails, códigos e documentos textuais;
- campos obrigatórios, opcionais e condicionais;
- unicidade e conflitos com estudantes já cadastrados;
- permissões e escopo da academia autenticada.

### 3.2 Criar status de pendência documental

Estudantes cadastrados sem arquivos devem ficar em um status específico que indique que ainda aguardam arquivamento/carregamento de documentos para ativação da conta.

Esse status deve:

1. ser distinto de ativo;
2. impedir que o estudante seja tratado como plenamente ativo em listagens, autenticação, matrícula, operações acadêmicas ou rotinas automáticas que exijam documentos completos;
3. permitir consulta administrativa pela academia;
4. permitir upload posterior dos documentos pela rota específica;
5. ser documentado como parte do ciclo de vida do estudante;
6. possuir transição clara para ativo somente após validação e armazenamento dos documentos obrigatórios.

### 3.3 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. cadastro em lote JSON sem documentos;
2. aceitação apenas dos campos textuais válidos do cadastro singular;
3. rejeição dos mesmos campos textuais inválidos rejeitados por `POST /academia/estudante/register`;
4. rejeição de arquivos quando `com_arquivo: false`;
5. criação do estudante com status de pendência documental;
6. estudante pendente não ser tratado como ativo;
7. persistência dos dados textuais necessários para exigir documentos posteriormente;
8. transição para ativo somente depois da rota de upload posterior.

---

# 4. Criar rota específica para carregar documentos de estudantes pendentes

## Objetivo

Criar uma rota dedicada para upload posterior dos documentos de estudantes cadastrados em massa sem arquivos, respeitando exatamente as regras de manejo documental da rota `POST /academia/estudante/register`.

## Regra de negócio

A nova rota deve:

1. receber os documentos de um estudante previamente cadastrado em lote sem arquivos;
2. aceitar apenas estudantes em status de pendência documental;
3. validar documentos com as mesmas regras de `POST /academia/estudante/register`;
4. armazenar documentos no mesmo padrão do cadastro singular;
5. ativar ou liberar o estudante somente após todos os documentos obrigatórios serem enviados, validados e arquivados;
6. impedir upload para estudante inexistente, de outra academia, já ativo, inativo, arquivado ou em status incompatível;
7. retornar erros padronizados para documentos ausentes, inválidos, duplicados ou incompatíveis;
8. registrar auditoria/metadados conforme os fluxos existentes de documentos.

## Escopo obrigatório

### 4.1 Definir contrato da nova rota

Criar e documentar uma rota específica para completar documentos de estudantes pendentes, por exemplo:

```text
POST /academia/estudante/{codigo_estudante}/documentos
```

O nome final da rota deve seguir as convenções reais do projeto, mas precisa ser exclusivo para o carregamento documental posterior desses estudantes e usar o código do estudante como referência do recurso, não um identificador genérico.

### 4.2 Cobrar BI conforme campos textuais preenchidos

A rota deve exigir Bilhetes de Identidade de acordo com os campos textuais informados no cadastro em lote sem arquivos:

1. se foi informado apenas o código/número do BI do responsável, exigir apenas o arquivo de BI do responsável;
2. se foi informado apenas o código/número do BI do estudante, exigir apenas o arquivo de BI do estudante;
3. se foram informados os códigos/números de BI do estudante e do responsável, exigir ambos os arquivos;
4. se nenhum BI textual foi informado e a regra singular permitir esse cenário, não exigir BI por esse critério específico;
5. se a regra singular tornar algum BI obrigatório por idade, nível, responsável ou outro campo, manter essa obrigatoriedade;
6. rejeitar arquivo de BI que não corresponda ao documento textual previamente cadastrado quando houver validação de correspondência disponível.

### 4.3 Garantir transição de status segura

Após upload e validação dos documentos, o backend deve:

1. verificar se todos os documentos obrigatórios foram arquivados;
2. atualizar o status do estudante para ativo ou para o próximo status definido no fluxo real somente quando a documentação estiver completa;
3. manter o status de pendência documental se faltar qualquer documento obrigatório;
4. impedir ativações parciais por falha de upload, falha de storage ou inconsistência de metadados;
5. registrar histórico, logs ou eventos de transição conforme o padrão do domínio.

### 4.4 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. upload posterior de documentos para estudante pendente;
2. rejeição de upload para estudante ativo ou em status incompatível;
3. rejeição de upload para estudante de outra academia;
4. cobrança apenas do BI do responsável quando só o BI textual do responsável foi informado;
5. cobrança apenas do BI do estudante quando só o BI textual do estudante foi informado;
6. cobrança dos dois BIs quando ambos foram informados;
7. não cobrança indevida de BI não informado, salvo regra singular condicional;
8. uso das mesmas validações de documentos do cadastro singular;
9. manutenção do status pendente quando a documentação estiver incompleta;
10. ativação ou liberação somente quando a documentação estiver completa;
11. rollback ou limpeza de arquivos quando a persistência falhar.

---

# 5. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir os novos contratos, modos de requisição, status, validações e rota de upload posterior.

## Escopo de documentação

Atualizar, quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio de estudantes;
- documentação de cadastro em massa/lote;
- documentação de uploads e storage;
- documentação de status e ciclo de vida do estudante;
- exemplos de payload JSON;
- exemplos de `multipart/form-data`;
- coleções de API;
- guias operacionais para academias;
- documentos de tarefas anteriores usados como referência ativa.

## Regras de documentação

A documentação deve declarar explicitamente que:

- a rota de cadastro em massa/lote suporta JSON e upload de arquivos;
- `com_arquivo` é obrigatório e define o modo da requisição;
- `com_arquivo: true` exige `multipart/form-data` e aplica as validações documentais das rotas singulares;
- `com_arquivo: false` aceita apenas campos textuais em JSON;
- o lote sem arquivos segue as validações JSON de `POST /academia/estudante/register`;
- estudante cadastrado sem arquivos não fica ativo;
- existe status específico de pendência documental;
- existe rota específica para carregar documentos de estudantes pendentes;
- BIs são exigidos na rota posterior conforme os BIs textuais preenchidos no cadastro sem arquivos;
- exemplos de sucesso e erro devem cobrir os dois modos de entrada;
- OpenAPI/Swagger deve descrever corretamente `application/json` e `multipart/form-data`.

---

# 6. Fora de escopo

- Criar regras documentais exclusivas para o lote que divirjam do cadastro singular ou da solicitação de matrícula.
- Ativar estudante cadastrado em lote sem documentos.
- Aceitar arquivos quando `com_arquivo` for `false`.
- Aceitar ausência de arquivos obrigatórios quando `com_arquivo` for `true`.
- Permitir upload posterior para estudante que não esteja em status de pendência documental.
- Cobrar BI do estudante ou do responsável sem base nos campos textuais informados, salvo obrigatoriedade já existente nas regras singulares.
- Criar aliases, wrappers de compatibilidade, fallbacks temporários ou contratos paralelos para formatos antigos.
- Alterar regras não relacionadas ao cadastro em massa/lote, cadastro singular de estudante, solicitação de matrícula, documentos ou status documental.

---

# 7. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. a rota de cadastro em massa/lote aceitar requisições com arquivos e requisições apenas JSON;
2. o campo `com_arquivo` for obrigatório e validado como booleano;
3. `com_arquivo: true` exigir e processar arquivos por `multipart/form-data`;
4. cada arquivo enviado for associado inequivocamente ao estudante correto;
5. arquivos órfãos, ausentes, duplicados ou ambíguos forem rejeitados;
6. lote com arquivos usar as mesmas validações das rotas singulares de cadastro de estudante e solicitação de matrícula;
7. `com_arquivo: false` aceitar e validar apenas campos textuais;
8. lote sem arquivos seguir as validações JSON de `POST /academia/estudante/register`;
9. estudante cadastrado sem arquivos ficar em status específico de pendência documental, não ativo;
10. existir rota específica para carregar documentos de estudantes pendentes;
11. a rota posterior validar, armazenar e auditar documentos com as mesmas regras de `POST /academia/estudante/register`;
12. a cobrança de BI na rota posterior respeitar os BIs textuais preenchidos no cadastro sem arquivos;
13. o estudante só ser ativado ou liberado quando todos os documentos obrigatórios estiverem válidos e arquivados;
14. OpenAPI/Swagger, documentação técnica, exemplos e coleções de API estiverem atualizados;
15. testes automatizados cobrirem os modos com arquivo, sem arquivo, upload posterior, status pendente, cobrança condicional de BI e erros principais;
16. o PR explicar claramente as mudanças de contrato, validação, storage, status, nova rota e documentação.
