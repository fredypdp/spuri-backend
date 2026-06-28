---
modificado: 2026-06-28 12:12
criado: 2026-06-15 18:06
---
# Cadastro de estudante escolar: BI do responsável obrigatório

## Objetivo

Consolidar, em todos os fluxos de entrada de estudante, as mesmas regras de validação de Bilhete de Identidade (BI) e documentos que já fazem parte da matrícula de estudante escolar, garantindo que o cadastro direto feito pela academia não fique menos restritivo do que a solicitação de matrícula.

A implementação deve impedir que estudantes escolares/fundamental/médio sejam criados ou aprovados sem BI do responsável e sem os documentos obrigatórios, além de evitar conflitos entre o BI do estudante, o BI do responsável e o BI de outros estudantes.

## Contexto e motivação

Esta tarefa foi criada quando o cadastro direto de estudante pela academia ainda não cobria os mesmos requisitos da solicitação de matrícula. Desde então, o fluxo de cadastro direto passou a exigir `multipart/form-data` e documentos acadêmicos/identificatórios, aproximando-se da matrícula por solicitação.

Mesmo assim, a regra de negócio precisa ficar explícita e verificável: **qualquer caminho que resulte na criação ou atualização de um estudante escolar deve aplicar o mesmo conjunto de validações**, independentemente de a origem ser:

- solicitação pública/de matrícula;
- aprovação de solicitação de matrícula;
- cadastro direto realizado pela academia;
- atualização cadastral que altere BI do estudante, BI do responsável ou documentos.

## Estado atual observado

Na solicitação de matrícula, o backend trabalha com campos textuais de identificação e anexos em PDF. Para estudante escolar, o fluxo deve exigir:

- `bilhete_identidade_responsavel` textual;
- PDF `bi_responsavel`;
- PDF `bi_estudante` ou, quando o estudante não tiver BI próprio, PDF `cedula_estudante`;
- documento acadêmico aplicável ao ano/nível informado, como certificado específico ou `declaracao` quando permitido.

No cadastro direto pela academia, o endpoint também deve operar por `multipart/form-data` e aplicar a mesma política documental acima antes de gravar o evento de criação do estudante.

A migração de unicidade do BI principal indica que `bilhete_identidade` do estudante é único quando preenchido, mas não torna o campo obrigatório no banco para todos os estudantes. Portanto, a obrigatoriedade do BI do responsável e dos documentos para estudantes escolares deve ser garantida na camada de domínio/handler antes da persistência.

## Regra de negócio a implementar ou confirmar

### Regra geral para BI do estudante

- `bilhete_identidade` do estudante, quando informado, deve continuar único conforme a regra global existente.
- Valores vazios ou compostos apenas por espaços devem ser tratados como ausentes após `trim`.
- Comparações entre BIs devem usar a normalização já adotada no sistema, no mínimo `trim` e comparação sem diferença por caixa quando aplicável.

### Regras para estudantes escolares/fundamental/médio

- `bilhete_identidade_responsavel` é obrigatório.
- PDF `bi_responsavel` é obrigatório.
- PDF `bi_estudante` é obrigatório quando houver BI do estudante.
- PDF `cedula_estudante` é obrigatório quando não houver BI do estudante e a política do produto permitir estudante escolar sem BI próprio.
- Documento acadêmico do ano/nível informado é obrigatório:
  - certificado específico quando aplicável;
  - ou `declaracao`, quando o certificado ainda não existir ou quando a regra do ano/nível permitir declaração.
- O BI do estudante, quando informado, não pode ser igual ao `bilhete_identidade_responsavel` do próprio estudante.
- O BI do responsável do estudante escolar não pode ser igual ao BI principal de outro estudante escolar/fundamental/médio.
- O BI do responsável pode repetir entre estudantes diferentes como BI de responsável, por exemplo, irmãos com o mesmo responsável.
- O BI do responsável pode coincidir com o BI principal de um estudante do ensino superior, quando esse estudante superior representa o responsável, desde que isso não viole outra regra global já existente.

## Escopo dos ajustes necessários

Aplicar ou revisar as validações nos pontos abaixo, mantendo mensagens de erro claras e consistentes:

1. **Criação da solicitação de matrícula escolar**
   - Validar BIs textuais.
   - Validar PDFs obrigatórios.
   - Validar documento acadêmico exigido pelo ano/nível.
   - Não criar solicitação pendente se alguma regra falhar.

2. **Aprovação da solicitação de matrícula**
   - Revalidar as regras antes de criar o estudante.
   - Revalidar conflito com estudantes já existentes, pois o cenário pode mudar entre a criação da solicitação e a aprovação.
   - Não aprovar a solicitação se os dados ficarem obsoletos ou conflitantes.

3. **Cadastro direto de estudante pela academia**
   - Exigir `multipart/form-data`; não aceitar criação direta sem documentos.
   - Aplicar a mesma regra documental da solicitação de matrícula.
   - Aplicar as mesmas regras de BI da solicitação de matrícula.
   - Gravar `EstudanteCriadoComVinculo` somente depois de todas as validações e uploads obrigatórios passarem.

4. **Atualização de dados cadastrais**
   - Revalidar quando a atualização alterar `bilhete_identidade`, `bilhete_identidade_responsavel`, ano/nível, curso ou documentos.
   - Impedir que uma atualização transforme um cadastro válido em um estudante escolar sem BI/documentos obrigatórios.

## Validações obrigatórias

- Estudante escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Estudante escolar com `bilhete_identidade_responsavel` vazio após `trim`: deve falhar.
- Estudante escolar sem PDF `bi_responsavel`: deve falhar.
- Estudante escolar com `bilhete_identidade` informado, mas sem PDF `bi_estudante`: deve falhar, exceto se a regra existente aceitar `cedula_estudante` como substituto documental.
- Estudante escolar sem BI próprio e sem PDF `cedula_estudante`: deve falhar.
- Estudante escolar sem certificado aplicável ou `declaracao`: deve falhar.
- Estudante escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Estudante escolar com BI do responsável igual ao BI principal de outro estudante escolar/fundamental/médio: deve falhar.
- Estudante escolar com BI do responsável repetido como BI do responsável de outro estudante: deve passar.
- Cadastro direto pela academia deve aplicar as mesmas regras da solicitação de matrícula.
- Aprovação de solicitação de matrícula deve revalidar as regras antes de criar o estudante.

## Fluxo operacional na solicitação de matrícula

1. Solicitante informa dados do estudante.
2. Backend identifica se a matrícula é escolar/fundamental/médio ou superior com base nos campos acadêmicos informados.
3. Se for estudante escolar, solicitante informa `bilhete_identidade_responsavel`.
4. Solicitante envia PDF `bi_responsavel`.
5. Solicitante informa BI do estudante e envia PDF `bi_estudante`, ou envia `cedula_estudante` quando a política do produto permitir estudante escolar sem BI próprio.
6. Solicitante envia certificado acadêmico aplicável ou `declaracao` conforme regra do ano/nível.
7. Backend normaliza os BIs usados na comparação.
8. Backend valida que o BI do estudante não é igual ao BI do responsável.
9. Backend valida que o BI do responsável não é igual ao BI principal de outro estudante escolar/fundamental/médio, preservando a possibilidade de responsável vinculado a estudante superior quando permitido.
10. Solicitação fica pendente somente se todas as regras forem atendidas.
11. Ao aprovar, backend reexecuta as validações e cria o estudante com os mesmos dados validados.

## Fluxo operacional no cadastro direto pela academia

1. Academia chama o endpoint de cadastro de estudante usando `multipart/form-data`.
2. Backend lê campos textuais e arquivos enviados.
3. Backend identifica o nível do estudante com base nos campos acadêmicos e nas regras existentes do domínio.
4. Se for estudante escolar, exige `bilhete_identidade_responsavel`, PDF `bi_responsavel`, PDF `bi_estudante` ou `cedula_estudante`, e certificado/declaração aplicável.
5. Backend normaliza BIs para comparação.
6. Backend valida que o BI do estudante, quando informado, é diferente do BI do responsável.
7. Backend valida que o BI do responsável não pertence como BI principal a outro estudante escolar/fundamental/médio.
8. Backend realiza upload dos documentos obrigatórios.
9. Backend grava evento `EstudanteCriadoComVinculo` somente após validação e upload bem-sucedidos.

## Testes recomendados

### Solicitação de matrícula

- Solicitação escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Solicitação escolar com `bilhete_identidade_responsavel` contendo apenas espaços: deve falhar.
- Solicitação escolar sem PDF `bi_responsavel`: deve falhar.
- Solicitação escolar com BI do estudante informado e sem PDF `bi_estudante`: deve falhar, salvo substituição aceita por regra existente.
- Solicitação escolar sem BI próprio e sem PDF `cedula_estudante`: deve falhar.
- Solicitação escolar sem certificado aplicável e sem `declaracao`: deve falhar.
- Solicitação escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Solicitação escolar com BI do responsável igual ao BI principal de outro estudante escolar/fundamental/médio: deve falhar.
- Solicitação escolar com BI do responsável igual ao BI principal de estudante superior: validar conforme regra de responsável superior e regras globais existentes.
- Solicitação escolar com BI do responsável repetido entre irmãos: deve passar.

### Cadastro direto pela academia

- Cadastro direto escolar enviado sem `multipart/form-data`: deve falhar.
- Cadastro direto escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Cadastro direto escolar sem PDF `bi_responsavel`: deve falhar.
- Cadastro direto escolar com BI próprio e sem PDF `bi_estudante`: deve falhar, salvo substituição aceita por regra existente.
- Cadastro direto escolar sem BI próprio e sem PDF `cedula_estudante`: deve falhar.
- Cadastro direto escolar sem certificado aplicável e sem `declaracao`: deve falhar.
- Cadastro direto escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Cadastro direto escolar com BI do responsável igual ao BI principal de outro estudante escolar/fundamental/médio: deve falhar.
- Cadastro direto escolar com todos os documentos e BIs válidos: deve passar e criar `EstudanteCriadoComVinculo`.

### Aprovação e atualização

- Aprovação de solicitação deve falhar se, entre a criação e a aprovação, outro estudante escolar/fundamental/médio tiver sido criado com BI principal igual ao BI do responsável da solicitação.
- Aprovação de solicitação deve falhar se os documentos obrigatórios da solicitação estiverem ausentes ou inválidos.
- Atualização cadastral deve falhar se remover `bilhete_identidade_responsavel` de estudante escolar.
- Atualização cadastral deve falhar se remover documentos obrigatórios ou alterar nível/ano sem manter documentos compatíveis.
- Atualização cadastral deve permitir BI do responsável repetido como BI de responsável de outro estudante.
