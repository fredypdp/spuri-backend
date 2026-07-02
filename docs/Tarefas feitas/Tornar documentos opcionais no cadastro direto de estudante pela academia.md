---
modificado: 2026-07-02 22:10
criado: 2026-06-29 00:00
---
# Tornar documentos opcionais no cadastro direto de estudante pela academia (feito)

## Objetivo

Remover a obrigatoriedade de envio de documentos no cadastro direto de estudante feito pela academia, permitindo que a academia crie o estudante mesmo quando nenhum anexo for enviado.

A mudança deve preservar as validações de consistência dos documentos quando eles forem informados, mas não deve bloquear a criação do estudante pela ausência de `bi_estudante`, `bi_responsavel`, `cedula_estudante`, `declaracao` ou certificados acadêmicos.

## Contexto e motivação

O cadastro direto de estudante pela academia foi aproximado do fluxo de solicitação de matrícula e passou a exigir documentos obrigatórios em `multipart/form-data`. Essa regra garante completude documental, mas também impede que a academia cadastre rapidamente estudantes quando os documentos ainda não estão disponíveis no momento da criação.

A nova regra de produto é flexibilizar esse fluxo: no cadastro direto pela academia, os documentos devem ser opcionais. A academia poderá criar o estudante com os dados cadastrais e acadêmicos necessários, deixando os anexos para envio posterior ou para outro processo de regularização documental.

Essa alteração deve afetar apenas o cadastro direto pela academia. A solicitação de matrícula pública e a aprovação de solicitações devem manter as regras documentais próprias, salvo decisão explícita em outra tarefa.

## Regra de negócio a implementar

### Regra geral

- No cadastro direto de estudante pela academia, nenhum documento deve ser obrigatório.
- A ausência de documentos não deve impedir a criação do estudante.
- O backend deve aceitar cadastro direto com `multipart/form-data` sem anexos.
- O backend deve continuar aceitando e persistindo documentos quando a academia os enviar.
- Documentos enviados continuam sujeitos às validações técnicas existentes, como tipo de ficheiro, tamanho máximo e nomes de campos permitidos.
- As validações cadastrais não documentais devem continuar obrigatórias, incluindo dados pessoais, vínculo acadêmico, regras de curso/ano, regras de turma e regras de Bilhete de Identidade textual quando aplicáveis.

### Documentos que deixam de ser obrigatórios no cadastro direto

No cadastro direto feito pela academia, os seguintes anexos devem passar a ser opcionais:

- `bi_estudante`
- `bi_responsavel`
- `cedula_estudante`
- `declaracao`
- `certificado_6_ano_fundamental`
- `certificado_9_ano_fundamental`
- `certificado_ensino_medio`

A ausência desses campos não deve gerar erro de validação no cadastro direto.

### Validação quando documentos forem enviados

Mesmo opcionais, os documentos enviados devem continuar seguindo as regras técnicas do sistema:

- O ficheiro deve ser PDF válido quando o campo representar documento PDF.
- O ficheiro deve respeitar o limite de tamanho já configurado.
- O nome do campo deve ser reconhecido pelo backend.
- O documento deve ser armazenado no mesmo formato de metadados já usado para documentos de estudante.
- Falhas de upload ou persistência devem continuar abortando a criação, com limpeza de ficheiros parcialmente enviados quando aplicável.

## Escopo dos ajustes necessários

### Cadastro direto de estudante pela academia

Revisar o handler responsável pelo cadastro direto para separar duas categorias de validação:

1. **Validações obrigatórias de cadastro**, que devem continuar bloqueando a criação:
   - nome e demais dados pessoais exigidos;
   - nível/ano/curso informado de forma consistente;
   - existência e vínculo da academia;
   - regras de turma e ano letivo;
   - unicidade e consistência dos campos textuais de BI quando preenchidos;
   - demais regras de domínio já existentes para criação do estudante.

2. **Validações documentais**, que não devem mais exigir presença mínima de anexos:
   - não exigir documento acadêmico aplicável;
   - não exigir `bi_responsavel` em PDF;
   - não exigir `bi_estudante` em PDF quando houver BI textual do estudante;
   - não exigir `cedula_estudante` quando o estudante não tiver BI textual;
   - apenas validar os ficheiros efetivamente enviados.

### Reutilização de regras compartilhadas

Se o cadastro direto reutiliza a mesma função de documentos obrigatórios da solicitação de matrícula, a implementação deve evitar alterar essa função de forma global sem critério. O comportamento recomendado é introduzir um parâmetro, modo ou função específica para indicar o contexto de validação:

- contexto `solicitacao_matricula`: mantém documentos obrigatórios conforme regra atual;
- contexto `cadastro_direto_academia`: documentos opcionais, valida somente anexos enviados.

Isso evita regressões no fluxo de solicitação de matrícula.

### Persistência

Quando documentos forem enviados no cadastro direto, manter a persistência já existente:

```json
{
  "documentos": {
    "bi_estudante": {
      "path": "ACA001/estudantes/EST20260001/documentos/bi_estudante.pdf",
      "file_url": "https://...",
      "download_url": "https://..."
    }
  }
}
```

Quando nenhum documento for enviado, o estudante deve ser criado com mapa de documentos vazio, nulo ou omitido conforme o padrão atual da projeção, desde que as consultas não quebrem.

## Compatibilidade com clientes existentes

- Clientes que já enviam `multipart/form-data` com documentos devem continuar funcionando.
- Clientes que enviam `multipart/form-data` apenas com campos textuais devem passar a funcionar.
- Se ainda houver suporte a JSON puro no cadastro direto, confirmar se o JSON também deve ser aceito sem documentos. Caso a rota atual tenha sido migrada para exigir `multipart/form-data`, não é obrigatório reabrir suporte a JSON nesta tarefa.
- A documentação da API deve deixar claro que, no cadastro direto pela academia, documentos são opcionais.

## Fora de escopo

- Alterar a obrigatoriedade de documentos na solicitação pública de matrícula.
- Alterar a revalidação documental durante aprovação de solicitação de matrícula, salvo se o código compartilhado exigir ajuste para evitar regressão.
- Criar uma nova rotina de cobrança ou regularização documental posterior.
- Remover campos de documentos já existentes no modelo de estudante.
- Apagar documentos já persistidos em estudantes existentes.

## Validações obrigatórias após a mudança

- Cadastro direto sem qualquer documento deve criar o estudante quando os demais dados obrigatórios estiverem válidos.
- Cadastro direto sem `bi_responsavel` em PDF deve criar o estudante quando os demais dados obrigatórios estiverem válidos.
- Cadastro direto com BI textual do estudante, mas sem `bi_estudante` em PDF, deve criar o estudante quando os demais dados obrigatórios estiverem válidos.
- Cadastro direto sem BI textual do estudante e sem `cedula_estudante` deve criar o estudante se a ausência do BI textual for permitida pela regra cadastral atual.
- Cadastro direto sem `declaracao` e sem certificado acadêmico deve criar o estudante quando os demais dados obrigatórios estiverem válidos.
- Cadastro direto com documento enviado em formato inválido deve falhar.
- Cadastro direto com documento enviado acima do limite permitido deve falhar.
- Cadastro direto com documento válido deve persistir os metadados do documento no estudante.
- Solicitação de matrícula deve continuar exigindo os documentos que já eram obrigatórios antes desta mudança.

## Fluxo operacional proposto

1. Academia chama o endpoint de cadastro direto de estudante.
2. Backend autentica a academia.
3. Backend lê os campos textuais e identifica os ficheiros enviados, se houver.
4. Backend valida dados pessoais, acadêmicos, vínculo, curso, turma, ano letivo e BIs textuais conforme regras atuais.
5. Backend não calcula pendências documentais obrigatórias para este contexto, ou calcula apenas como informação não bloqueante.
6. Backend valida tipo e tamanho somente dos documentos efetivamente enviados.
7. Backend faz upload dos documentos enviados, se houver.
8. Backend cria o estudante e grava o evento com os documentos enviados ou sem documentos.
9. Projeção de estudantes expõe os documentos existentes sem assumir que todos os tipos estarão presentes.
10. Em caso de erro após upload parcial, backend remove os ficheiros já enviados.

## Impactos esperados

- A academia consegue cadastrar estudantes diretamente mesmo sem anexos disponíveis.
- O cadastro direto fica mais flexível e menos bloqueante para operações internas da academia.
- Documentos continuam suportados e auditáveis quando enviados.
- A solicitação de matrícula mantém seu rigor documental, evitando mudança acidental em outro fluxo.
- Consultas e projeções devem lidar com estudantes sem documentos cadastrados.

## Documentação da API

Atualizar a documentação do cadastro direto de estudante para indicar que:

- Os campos de documentos são opcionais no cadastro direto pela academia.
- O conteúdo enviado deve ser PDF válido.
- Os documentos continuam sendo retornados nas consultas quando existirem.
- Exemplos de requisição devem incluir um caso sem documentos e um caso com documentos opcionais.

## Testes recomendados

### Cadastro direto pela academia

- Cadastro direto com dados válidos e nenhum documento: deve passar.
- Cadastro direto com dados válidos e apenas `bi_estudante`: deve passar e persistir o documento.
- Cadastro direto com dados válidos e apenas `declaracao`: deve passar e persistir o documento.
- Cadastro direto com dados válidos e múltiplos documentos opcionais: deve passar e persistir todos os metadados.
- Cadastro direto com documento não PDF: deve falhar.
- Cadastro direto com documento acima do tamanho máximo: deve falhar.
- Cadastro direto com falha simulada após upload: deve limpar documentos enviados.
- Consulta do estudante criado sem documentos: deve retornar resposta válida, sem erro por mapa de documentos vazio ou ausente.
- Consulta do estudante criado com documentos opcionais: deve retornar os metadados dos documentos enviados.

### Regressão de solicitação de matrícula

- Solicitação de matrícula sem documento obrigatório: deve continuar falhando.
- Solicitação de matrícula com todos os documentos obrigatórios: deve continuar passando.
- Aprovação de solicitação de matrícula deve continuar usando as regras documentais específicas da solicitação.

### Projeções e rebuild

- Rebuild da projeção de estudantes deve restaurar corretamente estudante sem documentos.
- Rebuild da projeção de estudantes deve restaurar corretamente estudante com subconjunto parcial de documentos.
