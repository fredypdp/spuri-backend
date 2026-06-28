---
modificado: 2026-06-28 0:00
criado: 2026-06-28 0:00
---
# Cadastro direto de estudante pela academia: documentação completa como solicitação de matrícula

## Objetivo

Aproximar o cadastro direto de estudante feito pela academia do fluxo de solicitação de matrícula, exigindo dados e documentos semelhantes aos pedidos no `POST /solicitacao-matricula`. O objetivo é garantir que estudantes matriculados por caminhos diferentes fiquem com o mesmo nível de informação cadastral, validação documental e cobrança de requisitos obrigatórios.

## Estado atual observado

O fluxo de solicitação de matrícula é o cadastro mais completo: recebe dados pessoais e académicos, valida Bilhete de Identidade (BI) do estudante e do responsável, exige ficheiros PDF de documentos, salva os anexos em storage e mantém o mapa `documentos` na solicitação.

No cadastro direto pela academia, o estudante é criado imediatamente via evento `EstudanteCriadoComVinculo`, mas o fluxo tende a focar nos campos cadastrais necessários para criar o estudante e vinculá-lo à academia. Com isso, estudantes cadastrados diretamente podem ficar sem os mesmos documentos exigidos de quem entra por solicitação de matrícula, especialmente `bi_estudante`, `bi_responsavel`, `cedula_estudante`, `declaracao` e certificados aplicáveis.

## Regra de negócio a implementar

### Regra geral

- Todo estudante criado pela academia deve passar por uma validação documental equivalente à solicitação de matrícula.
- O backend deve exigir, no cadastro direto, os mesmos documentos obrigatórios que seriam exigidos para uma solicitação de matrícula do mesmo estudante, considerando nível académico, ano académico, curso e presença ou ausência de BI do estudante.
- A diferença entre os fluxos deve ser apenas operacional:
  - Na solicitação de matrícula, o pedido fica pendente até aprovação.
  - No cadastro direto pela academia, o estudante é criado imediatamente, mas somente depois de todos os dados e documentos obrigatórios serem aceitos.

### Documentos mínimos a alinhar

- `bi_responsavel`: obrigatório quando o fluxo documental exigir BI do responsável.
- `bi_estudante`: obrigatório quando o estudante tiver BI próprio e o backend exigir o anexo correspondente.
- `cedula_estudante`: obrigatória quando `bi_estudante` não for enviado ou quando a política atual permitir estudante sem BI próprio apenas com cédula.
- `declaracao`: obrigatória quando o certificado aplicável ao ano académico não for enviado.
- `certificado_6_ano_fundamental`: obrigatório quando aplicável ao ano/nível do estudante, salvo se a regra permitir substituição por `declaracao`.
- `certificado_9_ano_fundamental`: obrigatório quando aplicável ao ano/nível do estudante, salvo se a regra permitir substituição por `declaracao`.
- `certificado_ensino_medio`: obrigatório quando aplicável ao ano/nível do estudante, salvo se a regra permitir substituição por `declaracao`.

## Ajuste necessário

Alterar o endpoint de cadastro direto de estudante pela academia para aceitar `multipart/form-data`, ou criar uma rota complementar equivalente, mantendo compatibilidade se houver clientes que ainda enviam JSON puro.

O backend deve reutilizar a mesma matriz de regras documentais da solicitação de matrícula, evitando duas implementações divergentes. Sempre que a regra de documentos da solicitação mudar, o cadastro direto pela academia deve acompanhar a mesma regra.

## Campos e anexos esperados

### Dados cadastrais

Manter os campos atuais do cadastro direto, incluindo, quando aplicável:

- `nome`
- `genero`
- `data_nascimento`
- `email`
- `telefone`
- `bilhete_identidade`
- `bilhete_identidade_responsavel`
- `ano_escolar_fundamental`
- `ano_escolar_medio`
- `curso_medio_id`
- `ano_superior`
- `curso_superior_id`
- Dados de vínculo com turma, curso, ano letivo ou outros campos já suportados pelo cadastro direto.

### Ficheiros PDF

Aceitar os mesmos nomes de campos do fluxo de solicitação de matrícula:

- `bi_estudante`
- `bi_responsavel`
- `cedula_estudante`
- `declaracao`
- `certificado_6_ano_fundamental`
- `certificado_9_ano_fundamental`
- `certificado_ensino_medio`

Todos os ficheiros enviados devem ser PDFs válidos, respeitando o mesmo limite de tamanho já aplicado no fluxo de solicitação de matrícula.

## Persistência dos documentos

Os documentos enviados no cadastro direto devem ser persistidos em storage de forma rastreável, em diretório próprio do estudante, por exemplo:

```text
{codigo_academia}/estudantes/{codigo_estudante}/documentos/
```

ou outro caminho equivalente, desde que:

- Não misture documentos de solicitação pendente com documentos definitivos do estudante.
- Permita recuperar `path`, `file_url` e `download_url` de cada documento.
- Permita auditar quais documentos foram entregues no momento do cadastro.
- Permita remover os ficheiros enviados caso a criação do estudante falhe após upload.

## Modelo de dados recomendado

Criar ou reaproveitar um campo de documentos do estudante no evento e na projeção, por exemplo:

```json
{
  "documentos": {
    "bi_responsavel": {
      "path": "ACA001/estudantes/EST20260001/documentos/bi_responsavel.pdf",
      "file_url": "https://...",
      "download_url": "https://..."
    }
  }
}
```

Se o evento `EstudanteCriadoComVinculo` ainda não suporta documentos, adicionar os metadados necessários ao payload do evento e atualizar a projeção de estudantes para expor esses documentos nas rotas de consulta da academia e do admin.

## Validações obrigatórias

- Cadastro direto sem documento obrigatório para o perfil do estudante deve falhar.
- Cadastro direto com ficheiro não PDF deve falhar.
- Cadastro direto com PDF acima do limite permitido deve falhar.
- Cadastro direto sem `bi_responsavel`, quando obrigatório, deve falhar.
- Cadastro direto sem `cedula_estudante` quando não houver `bi_estudante` deve falhar.
- Cadastro direto sem `declaracao` e sem certificado aplicável deve falhar quando a regra exigir um dos dois.
- Cadastro direto deve aplicar as mesmas regras de BI da solicitação de matrícula, incluindo comparação normalizada entre `bilhete_identidade` e `bilhete_identidade_responsavel`.
- Se qualquer validação falhar depois de upload parcial, o backend deve limpar os documentos já enviados para evitar ficheiros órfãos.

## Fluxo operacional proposto

1. Academia envia cadastro direto em `multipart/form-data`, com dados do estudante e PDFs.
2. Backend autentica a academia e valida que ela pode criar estudante no nível/curso informado.
3. Backend identifica a regra documental aplicável usando os mesmos critérios da solicitação de matrícula.
4. Backend valida campos cadastrais, BI do estudante e BI do responsável.
5. Backend valida presença, tipo e tamanho dos PDFs obrigatórios.
6. Backend gera ou reserva o `codigo_estudante` necessário para montar o caminho dos documentos.
7. Backend faz upload dos documentos para o diretório definitivo do estudante.
8. Backend grava o evento `EstudanteCriadoComVinculo` incluindo metadados dos documentos.
9. Projeção de estudantes passa a armazenar e retornar os documentos cadastrados.
10. Se qualquer etapa após o upload falhar, backend remove os documentos enviados e retorna erro claro.

## Compatibilidade com clientes existentes

Caso existam integrações que usam JSON no cadastro direto atual, considerar uma das estratégias abaixo:

- Tornar a nova validação obrigatória em uma nova versão/rota e depreciar a rota antiga.
- Manter JSON apenas para cenários administrativos excepcionais, com campo explícito de justificativa e permissões mais restritas.
- Migrar a rota atual para `multipart/form-data` e comunicar breaking change aos clientes.

A decisão deve evitar que o fluxo antigo continue sendo uma forma de burlar os documentos obrigatórios.

## Impactos esperados

- Cadastro direto pela academia passa a ter o mesmo rigor documental da solicitação de matrícula.
- Estudantes cadastrados por caminhos diferentes ficam com dados e anexos equivalentes.
- A academia passa a conseguir consultar documentos de estudantes cadastrados diretamente, não apenas documentos de solicitações.
- O backend reduz inconsistências entre matrícula aprovada e cadastro direto.
- Documentação da API deve ser atualizada com payload `multipart/form-data`, campos, anexos, exemplos e mensagens de erro.

## Testes recomendados

- Cadastro direto com todos os documentos obrigatórios: deve criar estudante e persistir metadados dos documentos.
- Cadastro direto sem `bi_responsavel`, quando obrigatório: deve falhar.
- Cadastro direto sem `bi_estudante` e sem `cedula_estudante`: deve falhar.
- Cadastro direto sem certificado aplicável e sem `declaracao`: deve falhar quando a regra exigir um dos dois.
- Cadastro direto com `bi_estudante` não PDF: deve falhar.
- Cadastro direto com PDF acima do limite: deve falhar.
- Cadastro direto com BI do estudante igual ao BI do responsável: deve falhar.
- Cadastro direto com falha simulada após upload: deve remover os documentos já enviados.
- Cadastro direto deve salvar `path`, `file_url` e `download_url` dos documentos na projeção do estudante.
- Consulta de estudante pela academia deve retornar os documentos cadastrados diretamente.
- Aprovação de solicitação de matrícula deve continuar funcionando sem regressão.
- Rebuild da projeção de estudantes deve restaurar corretamente os documentos gravados no evento.
