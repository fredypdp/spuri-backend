---
criado: 2026-07-15 00:00
origem: auditoria de documentos acadêmicos do estudante
status: pendente
---

# Separar documentos acadêmicos de estudante por nível e ano

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que declarações e certificados do estudante nunca se substituam quando pertencerem a anos acadêmicos, níveis ou cursos diferentes. A regra deve valer em todas as rotas que manipulam documentos de estudante, incluindo cadastro pela academia, solicitação de matrícula, aprovação de solicitação que cria estudante, carregamento posterior de documentos pendentes, batch/importação e qualquer reenvio/correção documental. O backend deve armazenar os arquivos em caminhos únicos por escopo acadêmico e persistir os metadados em estrutura por nível e ano, versionável e consultável por tipo documental, nível, ano acadêmico e curso quando aplicável. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a código legado, aliases, wrappers de compatibilidade, fallbacks temporários ou respostas/documentos duplicados em chaves ambíguas.

## Contexto da auditoria

Foi executada auditoria no fluxo atual de documentos de matrícula/cadastro do estudante para verificar se existe mecanismo que separa documentos do mesmo tipo em níveis ou anos diferentes, especialmente `declaracao` e certificados.

A implementação atual não possui esse mecanismo ideal:

1. Os documentos do estudante são mantidos em `map[string]DocumentoMatricula`, portanto a chave documental é apenas o tipo/campo (`declaracao`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio`, etc.).
2. O upload de cadastro direto salva arquivos em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/{field}_{codigo_estudante}.pdf`, sem incluir nível, ano acadêmico, curso, ano letivo ou versão no caminho.
3. O upload para completar documentos pendentes usa o mesmo padrão `{field}_{codigo_estudante}.pdf`.
4. O aggregate e a projeção substituem o mapa completo de documentos quando `EstudanteDocumentosCompletados` é aplicado, em vez de mesclar por escopo ou preservar histórico documental.
5. A projeção `projection_estudantes.documentos` é um `JSONB` de metadados por campo simples, sem índice estrutural por nível/ano.
6. A validação já diferencia parcialmente o ano da `declaracao` por `ano_academico`, mas esse metadado fica dentro de um único objeto `declaracao`; uma nova declaração com outro ano continua ocupando a mesma chave lógica.
7. Os certificados são campos separados apenas por marcos (`certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio`), mas não há coleção histórica por ano/nível/curso para múltiplos documentos equivalentes ao longo do tempo.

Portanto, quando um estudante acumular declarações/certificados de anos ou níveis diferentes, o desenho atual tende a substituir os metadados anteriores na estrutura e a reutilizar caminhos de storage para o mesmo campo, o que não deve acontecer.

## Resumo executivo

| Item | Situação atual | Resultado esperado |
| --- | --- | --- |
| Estrutura em ledger/projeção | `map[string]DocumentoMatricula` por campo simples | `documentos` organizado por nível e ano acadêmico, preservando tipos documentais dentro do escopo |
| Storage | Caminho `{field}_{codigo_estudante}.pdf` | Caminho único por escopo acadêmico e versão |
| Declarações | Uma chave `declaracao` com `ano_academico` interno | Várias declarações coexistindo por ano acadêmico/nível |
| Certificados | Chaves fixas por tipo de certificado | Certificados coexistindo por nível, conclusão, curso/ano quando aplicável |
| Eventos | Evento substitui conjunto de documentos | Evento acrescenta/atualiza documento por escopo sem apagar histórico |
| Downloads | `campo` simples na URL | Rota ou query com identificador documental/escopo inequívoco |
| Testes | Cobrem obrigatoriedade/validação, não coexistência histórica | Regressões provando que documentos antigos não são substituídos |

---

# 1. Criar modelo documental acadêmico normalizado

## Objetivo

Garantir que cada documento acadêmico do estudante seja identificado por uma chave composta estável, evitando colisões entre arquivos do mesmo tipo pertencentes a anos, níveis, cursos ou versões diferentes.

## Regra de negócio

A estrutura pública e persistida do campo `documentos` do estudante deve ser organizada por nível e ano acadêmico. O formato desejado deve seguir aproximadamente este desenho, abrangendo todos os níveis vigentes:

```json
{
  "documentos": {
    "identificacao": {
      "bi_estudante": { "path": "...", "file_url": "...", "download_url": "..." },
      "bi_responsavel": { "path": "...", "file_url": "...", "download_url": "..." }
    },
    "fundamental": {
      "1_ano_fundamental": {},
      "2_ano_fundamental": {
        "declaracao_1_ano_fundamental": { "path": "...", "file_url": "...", "download_url": "..." }
      }
    },
    "medio": {
      "1_ano_medio": {
        "certificado_9_ano_fundamental": { "path": "...", "file_url": "...", "download_url": "..." },
        "declaracao_9_ano_fundamental": { "path": "...", "file_url": "...", "download_url": "..." }
      }
    },
    "superior": {
      "1_ano_superior": {
        "certificado_ensino_medio": { "path": "...", "file_url": "...", "download_url": "..." },
        "declaracao_3_ano_medio": { "path": "...", "file_url": "...", "download_url": "..." }
      }
    }
  }
}
```

A implementação pode acrescentar metadados internos, identificadores e versões, mas não pode voltar ao formato ambíguo em que `declaracao` genérico ou `certificado_*` ficam no topo do mapa sem escopo acadêmico. Declarações devem ter chave explícita do nível/ano comprovado, como `declaracao_9_ano_fundamental`, `declaracao_3_ano_medio` ou `declaracao_1_ano_fundamental`, e não uma chave vaga `declaracao`.

### Regras explícitas de escopo e nomenclatura

1. `bi_estudante` e `bi_responsavel` são documentos gerais de identificação e **não** devem ficar dentro dos escopos acadêmicos `fundamental`, `medio` ou `superior`. Eles devem permanecer em escopo geral, como `identificacao`, ou estrutura equivalente sem associação a nível/ano acadêmico.
2. `1_ano_fundamental` não possui ano acadêmico anterior exigível; portanto, não deve ter declaração de entrada, declaração anterior ou qualquer comprovativo acadêmico obrigatório. O escopo pode existir vazio ou conter apenas metadados não documentais, mas não deve exigir `declaracao_*` nem certificado.
3. Não existe o tipo documental `declaracao_ensino_medio`. A declaração válida para comprovar conclusão/ano anterior ao ingresso no superior deve usar a nomenclatura explícita `declaracao_3_ano_medio`, acompanhando a regra atual de validação que compara a declaração com o ano acadêmico anterior (`3_ano_medio`) ao `1_ano_superior`.
4. A implementação deve remover qualquer uso novo de `declaracao_ensino_medio` em contratos, validações, handlers, DTOs, exemplos, OpenAPI/Swagger e documentação. Se o debug encontrar uso existente dessa nomenclatura em código executável, a própria tarefa deve incluir a correção para `declaracao_3_ano_medio`, sem alias ou fallback legado.

Cada documento acadêmico de estudante deve possuir, no mínimo:

1. `id` ou `documento_id` estável;
2. `tipo` com escopo explícito para declarações e certificados (`declaracao_1_ano_fundamental`, `declaracao_9_ano_fundamental`, `declaracao_3_ano_medio`, `certificado_6_ano_fundamental`, `certificado_9_ano_fundamental`, `certificado_ensino_medio`, ou outro tipo vigente), proibindo o tipo genérico `declaracao` nos novos contratos;
3. `nivel` (`fundamental`, `medio`, `superior`, ou enum equivalente vigente), usado como primeira camada dentro de `documentos`;
4. `ano_academico`, usado como segunda camada dentro de cada nível quando aplicável;
5. `curso_id` quando aplicável para médio/superior;
6. `ano_letivo` quando necessário para rastreabilidade;
7. `versao` ou `sequencia` para permitir reenvio sem sobrescrever histórico;
8. `path`, `file_url`, `download_url` e metadados técnicos do arquivo;
9. timestamps e usuário/origem responsável pelo upload quando o padrão do projeto permitir.

## Debug de nomenclatura executado

Foi verificado que a nomenclatura `declaracao_ensino_medio` não aparece em código executável, migrations, validações ou handlers antes desta tarefa; a busca apontou apenas o próprio documento de tarefa que estava sendo ajustado. A implementação desta tarefa ainda deve manter uma verificação final em `internal/`, `migrations/`, documentação técnica e OpenAPI/Swagger para garantir que nenhum novo uso de `declaracao_ensino_medio` seja introduzido.

Com base na validação atual de ano acadêmico anterior, o ingresso em `1_ano_superior` tem como ano anterior `3_ano_medio`; por isso, a chave explícita correta da declaração é `declaracao_3_ano_medio`.

## Escopo obrigatório

### 1.1 Ajustar domínio e eventos

Atualizar aggregates, eventos e serializers para que documentos acadêmicos sejam representados como coleção de entradas documentais e não como mapa simples por campo.

O evento de novo documento deve acrescentar ou registrar documento por escopo. Não deve substituir todo o conjunto documental do estudante, salvo em fluxo explícito de correção auditável.

### 1.2 Ajustar projeção

Atualizar `projection_estudantes.documentos` para armazenar a estrutura por nível e ano acadêmico, permitindo múltiplos documentos do mesmo tipo em escopos diferentes sem colisão.

A estrutura deve permitir consultas como:

- todas as declarações do estudante, percorrendo todos os níveis e anos e preservando a chave explícita do nível comprovado;
- declaração de um ano acadêmico específico;
- certificados por nível;
- certificados por curso quando aplicável;
- documento atual/mais recente de um escopo;
- histórico completo de versões/documentos substituídos administrativamente.

### 1.3 Planejar migração de dados

Criar migration segura para transformar o JSON atual por campo simples em coleção normalizada.

A migração deve preservar todos os metadados existentes e atribuir escopo inferido somente quando o dado atual permitir. Quando não for possível inferir, marcar explicitamente como `escopo_desconhecido` ou campo equivalente, sem descartar documento.

---

# 2. Separar caminhos de storage por escopo acadêmico

## Objetivo

Impedir que uploads de documentos do mesmo tipo reutilizem o mesmo caminho físico no storage.

## Regra de negócio

O caminho do arquivo deve incluir componentes suficientes para ser único e rastreável, por exemplo:

```text
{codigo_academia}/estudantes/{codigo_estudante}/documentos/{nivel}/{ano_academico}/{tipo}/{documento_id}.pdf
```

Para documentos com curso, incluir o curso quando necessário:

```text
{codigo_academia}/estudantes/{codigo_estudante}/documentos/{nivel}/{curso_id}/{ano_academico}/{tipo}/{documento_id}.pdf
```

Não usar apenas `{field}_{codigo_estudante}.pdf` para declarações/certificados acadêmicos.

## Escopo obrigatório

### 2.1 Ajustar handlers de upload

Atualizar todos os fluxos e rotas que aceitam, movem, completam, aprovam, consultam ou baixam documentos de estudante. A regra de separação por nível e ano deve ser aplicada de forma uniforme em:

1. cadastro direto por academia;
2. completar documentos pendentes;
3. solicitação de matrícula;
4. aprovação de solicitação que cria estudante;
5. batch/importação que encaminhe documentos;
6. endpoints de listagem e download que retornam documentos do estudante;
7. qualquer endpoint futuro ou existente de reenvio/correção documental.

### 2.2 Garantir atomicidade

Falha ao gravar um documento novo não pode apagar documentos anteriores nem deixar metadados apontando para arquivo inexistente.

Se houver rollback de upload em lote, remover apenas arquivos recém-enviados naquele fluxo, nunca a pasta inteira do estudante quando ela já puder conter documentos antigos.

---

# 3. Atualizar contratos de consulta e download

## Objetivo

Permitir que clientes consultem e baixem documentos sem ambiguidade quando houver múltiplos documentos do mesmo tipo.

## Regra de negócio

As respostas de estudante, inventário documental e solicitações devem expor documentos no formato por nível e ano acadêmico, com identificador único e escopo acadêmico em cada entrada documental.

Downloads não devem depender apenas de `{campo}` quando existir mais de um documento do mesmo tipo. Usar `documento_id` ou rota/query inequívoca.

## Escopo obrigatório

### 3.1 Ajustar rotas de listagem

Atualizar as respostas de:

1. consulta própria do estudante;
2. consulta de estudante pela academia/admin;
3. inventário documental da academia;
4. consulta de solicitações de matrícula;
5. consulta/listagem de documentos pendentes;
6. qualquer endpoint usado pelo front end para listar documentos.

### 3.2 Ajustar rotas de download

Criar ou adaptar rotas para baixar por `documento_id` ou chave composta. Manter autorização atual por perfil, academia vinculada e dono do documento.

---

# 4. Atualizar validações acadêmicas

## Objetivo

Manter as regras atuais de obrigatoriedade de documentos acadêmicos usando a nova coleção normalizada.

## Regra de negócio

A validação deve localizar documentos por tipo e escopo, por exemplo:

1. declaração do ano acadêmico anterior esperado usando tipo explícito, por exemplo `declaracao_9_ano_fundamental` ou `declaracao_3_ano_medio`;
2. certificado do 6.º ano fundamental para ingresso no 7.º ano fundamental;
3. certificado do 9.º ano fundamental para ingresso no 1.º ano médio;
4. certificado do ensino médio para ingresso no 1.º ano superior;
5. documento correspondente ao curso quando o nível exigir curso.

A existência de documento de outro ano, nível ou curso não deve satisfazer a regra do escopo atual.

---

# 5. Atualizar testes

Adicionar ou ajustar testes cobrindo obrigatoriamente:

1. duas declarações de anos acadêmicos diferentes coexistem na projeção com chaves explícitas, sem usar `declaracao` genérico;
2. duas declarações de anos acadêmicos diferentes geram paths diferentes no storage;
3. certificado antigo não é sobrescrito ao adicionar certificado de outro nível;
4. completar documentos pendentes não substitui documentos já existentes fora do nível/ano completado;
5. aprovação de solicitação preserva documentos da solicitação com escopo normalizado;
6. download por identificador retorna o documento correto entre múltiplos do mesmo tipo;
7. validação rejeita declaração de nível/ano diferente do esperado mesmo havendo outra declaração válida para outro contexto;
8. rollback de upload remove apenas arquivos recém-criados;
9. migration preserva documentos legados;
10. serialização/deserialização do ledger preserva múltiplos documentos por tipo;
11. todas as rotas manipuladoras de documentos de estudante retornam e persistem a mesma estrutura por nível e ano.

---

# 6. Atualizar documentação

Atualizar documentação técnica, documentação de API, OpenAPI/Swagger, exemplos de payload e manuais do front end para refletir:

1. nova estrutura documental por nível e ano acadêmico no campo `documentos`;
2. paths de storage por escopo;
3. rotas de download por documento inequívoco;
4. regras de validação por nível/ano/curso;
5. comportamento esperado para histórico e reenvio documental;
6. inexistência de substituição automática entre documentos de anos/níveis diferentes;
7. exemplos completos para `fundamental`, `medio` e `superior`, incluindo a estrutura aproximada esperada em `documentos` e declarações com nomes explícitos por nível/ano, nunca apenas `declaracao`.
