---
criado: 2026-07-11 00:00
origem: solicitação do usuário
status: pendente
---

# Unificar validação de cadastro direto e solicitação de matrícula (feito)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que `POST /academia/estudante/register` e `POST /solicitacao-matricula` usem o mesmo código compartilhado para validar dados comuns de estudante, responsável, nível escolar/superior, telefones e documentos. Corrija a ordem do fluxo para que todo upload obrigatório de documentos seja concluído com sucesso antes de qualquer gravação no ledger. Não criar validações duplicadas por rota, aliases, fallbacks temporários ou caminhos alternativos que permitam uma rota aceitar dados que a outra rejeitaria para o mesmo contexto.

## Contexto

A rota `POST /solicitacao-matricula` parece ser hoje o fluxo mais completo e correto quanto às validações cadastrais e documentais. Ela aplica regras distintas para estudantes de nível escolar e de ensino superior, especialmente quanto à obrigatoriedade de telefone e documentação.

No entanto, em `POST /academia/estudante/register`, apenas o telefone do estudante está sendo usado como referência obrigatória. Esse comportamento está incorreto para o caso escolar, porque, nessa situação, o telefone do responsável deve ser obrigatório e o telefone do estudante deve ser opcional. No ensino superior, a regra é inversa: o telefone obrigatório é o do estudante.

Também há falha na validação e na ordem do upload de documentos. Antes de qualquer registro no ledger, os documentos exigidos devem ser enviados ao storage e validados com sucesso. Somente depois de todos os uploads obrigatórios terem sido concluídos deve ocorrer a gravação no ledger. Se o upload falhar, nenhuma escrita de estudante, solicitação, matrícula, vínculo ou evento correlato deve ser registrada.

A correção deve eliminar divergências entre rotas para dados que elas têm em comum. Quando ambas receberem as mesmas informações de estudante, responsável, nível, curso, telefone ou documentos, essas informações devem passar pelo mesmo código de validação, com parâmetros explícitos apenas para as diferenças reais de contexto.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Validações comuns | Centralizar em serviço/schema/validador compartilhado | `POST /academia/estudante/register` e `POST /solicitacao-matricula` não divergem para os mesmos dados |
| Telefone no nível escolar | Telefone do responsável obrigatório; telefone do estudante opcional | Cadastro direto e solicitação aplicam a mesma regra |
| Telefone no superior | Telefone do estudante obrigatório | Cadastro direto e solicitação aplicam a mesma regra |
| Documentos | Validar presença, tipo, tamanho e regras por contexto antes do ledger | Nenhuma gravação ocorre com documentos inválidos ou upload incompleto |
| Upload antes do ledger | Upload obrigatório concluído antes de registrar eventos | Falha de storage não cria estado inconsistente |
| Código duplicado | Proibido para regras comuns | Uma alteração futura na regra comum afeta as duas rotas com segurança |

---

# 1. Unificar validações comuns entre as duas rotas

## Objetivo

Garantir que `POST /academia/estudante/register` e `POST /solicitacao-matricula` passem pelo mesmo código de validação sempre que tratarem dados equivalentes.

## Regra de negócio

As duas rotas devem compartilhar validações para, no mínimo:

1. identificação do nível de ensino do estudante;
2. dados pessoais comuns do estudante;
3. dados comuns do responsável quando aplicável;
4. obrigatoriedade de telefone conforme nível;
5. consistência entre BI do estudante e BI do responsável;
6. documentos obrigatórios por nível, ano, curso e contexto;
7. tipo e tamanho dos arquivos enviados;
8. normalização dos campos usados para validação;
9. mensagens de erro padronizadas;
10. montagem de metadados documentais que serão persistidos após upload.

## Escopo obrigatório

### 1.1 Criar validador compartilhado

Extrair as validações comuns para uma camada reutilizável, por exemplo:

- schema compartilhado;
- serviço de domínio;
- caso de uso auxiliar;
- módulo de validação documental e cadastral;
- função pura de validação com entrada tipada.

A implementação deve impedir que cada rota mantenha uma cópia independente das mesmas regras.

### 1.2 Preservar diferenças reais de contexto

Diferenças entre as rotas devem ser explícitas e parametrizadas. Exemplos de contexto aceitáveis:

- `solicitacao_matricula`: cria solicitação pendente;
- `cadastro_direto_academia`: cria estudante diretamente vinculado à academia;
- política documental obrigatória ou opcional, se o produto vigente ainda diferenciar os fluxos;
- destino de storage e metadados persistidos.

Não usar o contexto para permitir divergência indevida nas regras de telefone, BI, nível, curso ou validações comuns.

### 1.3 Remover validações paralelas conflitantes

Auditar handlers, DTOs, schemas, middlewares e casos de uso das duas rotas e remover validações manuais duplicadas que possam divergir da regra compartilhada.

Se existir validação antiga em `POST /academia/estudante/register` exigindo apenas `telefone` do estudante, ela deve ser substituída pela regra centralizada.

---

# 2. Corrigir obrigatoriedade de telefone por nível de ensino

## Objetivo

Aplicar nas duas rotas a mesma regra já considerada correta pela solicitação de matrícula.

## Regra de negócio

### 2.1 Nível escolar

Para estudantes de nível escolar, incluindo fundamental e médio quando aplicável:

1. telefone do responsável é obrigatório;
2. telefone do estudante é opcional;
3. ausência de telefone do responsável deve gerar erro de validação;
4. presença de telefone do estudante não substitui o telefone obrigatório do responsável;
5. se ambos forem enviados, ambos devem ser validados quanto ao formato aceito pelo backend.

### 2.2 Ensino superior

Para estudantes do ensino superior:

1. telefone do estudante é obrigatório;
2. telefone do responsável não deve ser obrigatório;
3. ausência de telefone do estudante deve gerar erro de validação;
4. telefone do responsável, quando enviado, deve ser validado tecnicamente, mas não deve bloquear por ausência.

### 2.3 Campos aceitos

Mapear explicitamente os nomes atuais dos campos nas duas rotas, incluindo variações já suportadas pelo contrato, e normalizar para um modelo interno único antes da validação.

O contrato final deve deixar claro qual campo representa:

- telefone do estudante;
- telefone do responsável;
- nível escolar/superior usado para decidir obrigatoriedade.

---

# 3. Validar documentos corretamente antes de gravar no ledger

## Objetivo

Impedir que qualquer registro seja gravado no ledger antes de os documentos obrigatórios terem sido validados e enviados com sucesso.

## Regra de negócio

Antes de qualquer gravação no ledger, o backend deve:

1. identificar documentos obrigatórios para o contexto e perfil do estudante;
2. validar presença dos documentos obrigatórios;
3. validar que os arquivos enviados usam campos permitidos;
4. validar tipo de arquivo, incluindo PDF quando aplicável;
5. validar tamanho máximo conforme regra global vigente;
6. executar upload dos arquivos para o storage correto;
7. confirmar sucesso de todos os uploads obrigatórios;
8. montar metadados finais dos documentos enviados;
9. somente então prosseguir com evento, comando, registro ou append no ledger.

Se qualquer validação ou upload falhar, a operação inteira deve falhar antes do ledger.

## Escopo obrigatório

### 3.1 Reordenar o fluxo operacional

A ordem mínima esperada para as duas rotas é:

1. autenticar e autorizar a chamada, quando aplicável;
2. normalizar payload e arquivos recebidos;
3. validar dados cadastrais comuns pelo validador compartilhado;
4. validar regra de telefone por nível;
5. validar regra documental;
6. fazer upload dos documentos exigidos e enviados;
7. em caso de sucesso total do upload, preparar metadados;
8. gravar no ledger com metadados documentais consistentes;
9. atualizar projeções ou retornar resposta conforme o fluxo atual.

### 3.2 Evitar estados inconsistentes

Não pode existir cenário em que:

- estudante seja criado sem documento obrigatório por falha de validação;
- solicitação seja criada sem documento obrigatório por falha de validação;
- evento seja gravado no ledger e o upload falhe depois;
- metadado documental seja persistido apontando para arquivo que não foi enviado;
- upload parcial deixe arquivos órfãos sem tentativa de limpeza quando uma etapa posterior falhar.

### 3.3 Limpeza em falhas após upload parcial

Se algum upload parcial ocorrer e uma etapa subsequente falhar antes ou durante a gravação final, o backend deve remover ou marcar para remoção os arquivos já enviados, conforme o mecanismo de storage vigente.

A falha de limpeza deve ser registrada para auditoria sem mascarar o erro principal retornado ao cliente.

---

# 4. Atualizar contratos e documentação

## Objetivo

Garantir que a documentação reflita que as duas rotas usam validação comum para dados comuns e que a ordem correta é upload antes do ledger.

## Escopo de documentação

Atualizar, quando existirem:

- OpenAPI/Swagger;
- documentação técnica das rotas;
- exemplos de `multipart/form-data`;
- guias de upload e storage;
- documentação de erros de validação;
- testes de contrato;
- coleções de API.

## Regras de documentação

A documentação deve declarar explicitamente que:

- no nível escolar, telefone do responsável é obrigatório e telefone do estudante é opcional;
- no ensino superior, telefone do estudante é obrigatório;
- documentos obrigatórios são validados antes da gravação no ledger;
- upload de documentos obrigatórios ocorre antes do ledger;
- falha de upload impede qualquer registro no ledger;
- `POST /academia/estudante/register` e `POST /solicitacao-matricula` compartilham a mesma validação para dados comuns.

---

# 5. Testes obrigatórios

## Cadastro direto pela academia

Adicionar ou ajustar testes para `POST /academia/estudante/register` cobrindo:

1. estudante escolar sem telefone do responsável deve falhar, mesmo com telefone do estudante;
2. estudante escolar com telefone do responsável e sem telefone do estudante deve passar quando os demais dados estiverem válidos;
3. estudante superior sem telefone do estudante deve falhar;
4. estudante superior com telefone do estudante deve passar quando os demais dados estiverem válidos;
5. documento obrigatório ausente deve falhar antes do ledger;
6. documento inválido deve falhar antes do ledger;
7. falha simulada de upload deve impedir qualquer gravação no ledger;
8. upload bem-sucedido deve gravar no ledger com metadados documentais corretos.

## Solicitação de matrícula

Adicionar ou ajustar testes para `POST /solicitacao-matricula` cobrindo:

1. regra de telefone escolar continua exigindo telefone do responsável;
2. regra de telefone superior continua exigindo telefone do estudante;
3. documentos obrigatórios continuam sendo validados pelo mesmo módulo compartilhado;
4. falha de upload impede criação da solicitação no ledger;
5. sucesso de upload gera solicitação com metadados documentais corretos.

## Testes de unicidade do código de validação

Adicionar testes ou assertions estruturais que reduzam risco de regressão, por exemplo:

1. testes unitários diretamente sobre o validador compartilhado;
2. testes de integração das duas rotas usando os mesmos cenários base;
3. mocks/spies confirmando que o fluxo não chama o ledger antes de concluir uploads;
4. cobertura para campos equivalentes com nomes diferentes normalizados para o mesmo modelo interno.

---

# 6. Fora de escopo

- Alterar regras de negócio não relacionadas a dados comuns de estudante, responsável, telefones, documentos, upload e ledger.
- Criar uma terceira rota de cadastro.
- Permitir modo legado em que `POST /academia/estudante/register` continue exigindo apenas o telefone do estudante para nível escolar.
- Fazer gravação no ledger antes de concluir uploads obrigatórios.
- Manter duas implementações separadas para a mesma regra de telefone ou documento.
- Ignorar falhas de upload e criar registros pendentes de arquivo obrigatório.

---

# 7. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `POST /academia/estudante/register` e `POST /solicitacao-matricula` usarem o mesmo código para validações comuns;
2. estudante escolar exigir telefone do responsável nas duas rotas;
3. estudante escolar permitir telefone do estudante como opcional nas duas rotas;
4. estudante superior exigir telefone do estudante nas duas rotas;
5. documentos obrigatórios forem calculados e validados por código compartilhado ou parametrizado por contexto;
6. nenhum registro no ledger ocorrer antes do sucesso dos uploads obrigatórios;
7. falhas de validação ou upload não deixarem eventos, estudantes, solicitações ou vínculos parcialmente registrados;
8. uploads parciais forem limpos ou registrados para compensação quando uma etapa posterior falhar;
9. OpenAPI/Swagger e documentação técnica refletirem as regras corrigidas;
10. testes automatizados cobrirem telefone, documentos, ordem upload-ledger e regressão das duas rotas;
11. o PR explicar claramente como a validação foi centralizada e como a ordem de upload antes do ledger foi garantida.
