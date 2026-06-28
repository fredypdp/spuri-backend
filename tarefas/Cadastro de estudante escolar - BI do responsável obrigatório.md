---
modificado: 2026-06-28 3:03
criado: 2026-06-15 18:06
---
# Cadastro de estudante escolar: BI do responsável obrigatório

## Objetivo

Verificar e consolidar a regra de que estudantes do nível escolar devem possuir Bilhete de Identidade e telefone do responsável, especialmente no cadastro direto e na solicitação de matrícula.

## Estado atual observado

Na solicitação de matrícula, o backend exige `bilhete_identidade_responsavel` textual e PDF `bi_responsavel`. Também exige `bi_estudante` em PDF ou `cedula_estudante` quando não houver BI do estudante. Na aprovação da solicitação, o estudante é criado a partir dos dados da solicitação.

A migração de unicidade do BI principal indica que `bilhete_identidade` do estudante é único quando preenchido, mas não torna o campo obrigatório no banco para todos os estudantes.

## Regra de negócio recomendada

### Para nível escolar/fundamental/médio

- `bilhete_identidade_responsavel`: obrigatório.
- `telefone_responsavel`: obrigatório.
- Documento do estudante:
  - Se estudante possui BI: `bilhete_identidade` obrigatório e documento `bi_estudante` obrigatório.
  - Se estudante ainda não possui BI: permitir `cedula_estudante`, mas registrar explicitamente `sem_bi_estudante=true` e exigir regularização posterior.
- Se a regra do produto for “BI do estudante sempre obrigatório”, então remover a alternativa da cédula e bloquear sem `bilhete_identidade`.

### Para superior

- BI do estudante deve ser obrigatório.
- Responsável pode ser opcional, dependendo da idade/política institucional.

## Ajuste necessário

Hoje existe `telefone` no payload da solicitação, mas ele parece representar telefone do estudante/contato geral. A regra pede telefone do responsável. Para não misturar significados, criar campo explícito:

- `telefone_responsavel`

E manter `telefone` para contato principal do estudante, se necessário.

## Validações

- Telefone do responsável deve ser normalizado e validado para Angola quando aplicável.
- BI do responsável não pode ser igual ao BI do estudante quando ambos existirem.
- BI do estudante deve continuar único no sistema.
- BI do responsável pode repetir entre irmãos.
- Não aceitar strings vazias após `trim`.
- No cadastro direto pela academia, aplicar as mesmas regras da solicitação.

## Fluxo operacional na solicitação de matrícula

1. Solicitante informa dados do estudante.
2. Informa BI e telefone do responsável.
3. Envia PDF do BI do responsável.
4. Informa BI do estudante ou envia cédula quando permitido.
5. Backend valida campos e documentos antes de upload definitivo.
6. Solicitação fica pendente.
7. Ao aprovar, backend cria estudante com os mesmos dados validados.

## Fluxo operacional no cadastro direto

1. Academia chama endpoint de cadastro de estudante.
2. Backend identifica se é escolar ou superior com base no tipo da academia/curso.
3. Se escolar, exige BI/alternativa documental do estudante conforme política e telefone do responsável.
4. Backend grava evento `EstudanteCriadoComVinculo` somente após validação.

## Testes recomendados

- Solicitação escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Solicitação escolar sem PDF `bi_responsavel`: deve falhar.
- Solicitação escolar sem BI do estudante e sem cédula: deve falhar.
- Solicitação escolar com cédula e sem BI, se permitido: deve passar.
- Cadastro direto escolar sem telefone do responsável: deve falhar após implementação.
- BI do estudante duplicado: deve falhar.
- BI do responsável igual ao BI do estudante: deve falhar.
