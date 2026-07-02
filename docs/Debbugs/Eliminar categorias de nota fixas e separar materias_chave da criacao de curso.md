# Eliminar categorias de nota fixas e separar `materias_chave` da criação de curso

## Contexto

As duas documentações atuais descrevem contratos que não devem continuar existindo como regra do sistema:

1. categorias de nota fixas, obrigatórias, pré-definidas ou implícitas para academias;
2. criação de curso já recebendo `materias_chave` no request;
3. categoria de nota com campo/conceito de `status`.

Esta tarefa tem como objetivo orientar uma auditoria completa no código e na documentação para remover esses conceitos, corrigir os contratos públicos e ajustar a implementação onde for necessário.

## Objetivo principal

Eliminar totalmente do código o conceito de categorias de nota fixas ou pré-definidas para qualquer academia, independentemente do tipo/nível de ensino.

A partir desta tarefa:

- toda categoria de nota deve ser uma configuração explícita da academia;
- nenhuma academia deve receber, herdar, validar ou aceitar categorias de nota por causa de uma lista fixa no código;
- nenhuma regra de notas, fórmula, validação, handler, aggregate, projection, teste ou documentação deve depender de categorias fixas como `nota_escola`, `nota_exame`, `p1`, `p2` ou qualquer equivalente;
- qualquer categoria usada para lançar nota, compor fórmula ou validar regra deve existir como categoria configurada para a academia e, quando aplicável, para o ano académico correspondente;
- categoria de nota não deve ter conceito de `status`; se existir no código, banco, projection, response, request, DTO, schema, documentação ou teste, deve ser eliminado ou migrado corretamente.

## Mudança obrigatória em cursos e `materias_chave`

Remover `materias_chave` do request de criação de curso.

A criação de curso não pode exigir nem aceitar `materias_chave`, porque `materias_chave` só pode ser preenchido com IDs de matérias existentes, e matérias do médio são criadas conectadas a um curso. Portanto, o fluxo correto deve ser:

1. criar o curso médio sem `materias_chave`;
2. criar as matérias disciplinares do médio associadas ao curso;
3. adicionar ou atualizar as `materias_chave` do curso por rota específica.

Deve ser criada ou ajustada uma rota específica para configurar `materias_chave` em um curso, separada da criação do curso.

Essa rota deve validar, no mínimo:

- o curso existe, pertence à academia autenticada e não está deletado/inativo conforme o modelo real de cursos;
- `materias_chave` só pode ser configurado para cursos de `type='medio'`;
- todos os IDs informados referenciam matérias existentes, ativas e pertencentes à mesma academia;
- todas as matérias informadas pertencem ao mesmo curso médio;
- cada matéria informada pertence ao ano académico correto dentro da configuração enviada;
- não há IDs duplicados dentro do mesmo ano académico;
- não há entradas duplicadas para o mesmo ano académico;
- não há entrada com `ano_academico` vazio;
- não há lista vazia de matérias-chave para um ano académico configurado;
- não é possível configurar `materias_chave` antes de existirem matérias válidas para o curso;
- a resposta da rota reflete o estado atualizado do curso.

## Auditoria obrigatória no código

Antes de implementar correções, fazer uma depuração/auditoria completa do código para identificar todos os pontos impactados.

A auditoria deve procurar, classificar e corrigir ocorrências relacionadas a:

- categorias fixas;
- categorias pré-definidas;
- categorias obrigatórias por tipo de academia ou nível de ensino;
- listas hardcoded de categorias de nota;
- `status` em categoria de nota;
- criação de curso aceitando `materias_chave`;
- edição de curso aceitando `materias_chave`, se a nova regra definir que edição geral também não deve configurar esse campo;
- batch/importação assíncrona de cursos aceitando `materias_chave` fora da rota específica;
- testes que assumem categorias fixas ou criação de curso com `materias_chave`;
- documentação que descreve categorias fixas, `status` em categoria de nota ou `materias_chave` no request de criação de curso.

Pesquisar no mínimo pelos termos:

- `categoriasEscolarFixas`;
- `categoriasSuperiorFixas`;
- `categorias fixas`;
- `categoria fixa`;
- `pré-definida`;
- `predefinida`;
- `nota_escola`;
- `nota_exame`;
- `p1`;
- `p2`;
- `status` próximo de categoria de nota;
- `materias_chave`;
- `MateriasChave`;
- `materiasChave`.

Não basta listar ocorrências: cada ocorrência deve ser classificada como bug ativo, dado legado/migration histórica aceitável, documentação a corrigir, teste a atualizar ou ocorrência ainda válida no novo modelo.

## Escopo mínimo de arquivos e camadas

Auditar e ajustar, no mínimo:

1. rotas registradas no servidor;
2. handlers de notas e categorias de nota;
3. handlers de cursos;
4. handlers de matérias disciplinares;
5. handlers de avaliação final e regras de avaliação final;
6. batch/async/importações que criem ou atualizem cursos, categorias, notas ou regras;
7. aggregates de academia, curso, estudante/notas e matéria disciplinar;
8. eventos de domínio relacionados;
9. projections de categorias de nota, cursos, notas, matérias e avaliação final;
10. migrations e schema das projections;
11. testes unitários e de handlers;
12. documentação funcional;
13. documentação da API.

## Regras esperadas para categorias de nota

Após a correção:

- categorias de nota são sempre cadastradas explicitamente pela academia;
- não há categorias criadas automaticamente pelo backend por tipo de academia;
- não há validação que permita categoria ausente apenas por ela estar em uma lista fixa;
- não há validação que rejeite categoria por pertencer a uma lista fixa de outro nível;
- lançamento de nota valida a categoria contra as categorias cadastradas da academia e contra os anos académicos aplicáveis, quando essa relação existir;
- regras de avaliação final validam `categorias_envolvidas` apenas contra categorias cadastradas da academia aplicáveis aos anos da regra;
- fórmulas não dependem de nomes hardcoded, apenas das categorias explicitamente referenciadas e cadastradas;
- categoria de nota não possui `status` em request, response, projection ativa, documentação ou regra de negócio.

## Regras esperadas para curso e `materias_chave`

Após a correção:

- `POST /academia/cursos` não aceita `materias_chave`;
- a documentação de criação de curso não mostra `materias_chave` no request;
- a criação de curso médio é possível antes da criação das matérias do curso;
- existe rota específica para adicionar/substituir/remover/configurar `materias_chave` de curso médio;
- a rota específica só aceita matérias existentes e já associadas ao curso;
- leituras e listagens de curso podem continuar retornando `materias_chave` como estado configurado, se esse campo fizer parte do contrato público do curso;
- avaliação final do médio continua usando `materias_chave` do curso/ano, mas deve lidar com curso ainda não configurado com erro claro quando a avaliação exigir essa configuração;
- nenhum fluxo alternativo permite gravar `materias_chave` durante a criação inicial do curso.

## Atualização obrigatória das duas documentações

Atualizar as duas documentações principais:

- `docs/Spuri - Documentação.md`;
- `docs/Spuri - API.md`.

As atualizações devem ocorrer nas secções que documentam os dados correspondentes, incluindo obrigatoriamente:

1. secção de categoria de nota, especialmente `### 2.13 Categoria de Nota`, removendo `status` e qualquer indicação de categoria fixa/pré-definida;
2. estrutura/entidade de curso, removendo `materias_chave` do request de criação;
3. documentação da rota de criação de curso;
4. documentação da nova rota específica de configuração de `materias_chave`;
5. documentação de notas, fórmulas e regras de avaliação final quando citarem categorias fixas ou valores pré-definidos;
6. exemplos JSON que ainda mostrem `materias_chave` na criação de curso;
7. exemplos JSON que mostrem `status` em categoria de nota;
8. qualquer tabela de campos que indique categorias de nota como fixas, automáticas, obrigatórias por tipo de academia ou pré-definidas.

A documentação deve deixar claro que categorias de nota são configuráveis pela academia e que `materias_chave` é configurado depois da criação do curso, por rota própria.

## Critérios de aceite

- Não existe mais código ativo com lista de categorias de nota fixas/pré-definidas.
- Não existe mais fallback, permissão ou rejeição baseada em categoria fixa por tipo de academia.
- Categoria de nota não possui `status` no contrato público nem no modelo ativo.
- `POST /academia/cursos` rejeita ou ignora de forma validada qualquer `materias_chave` enviado, conforme o padrão de validação do projeto, e a documentação não instrui o cliente a enviá-lo.
- Existe rota específica para configurar `materias_chave` de curso médio após a criação das matérias.
- A nova rota valida que as matérias pertencem ao curso e ao ano académico informado.
- Avaliação final do médio usa apenas `materias_chave` configuradas no curso e falha com erro claro se a configuração necessária não existir.
- As duas documentações foram atualizadas nas secções correspondentes.
- Testes cobrem remoção de categorias fixas, remoção de `status` em categoria de nota, criação de curso sem `materias_chave` e configuração de `materias_chave` pela rota específica.

## Entregáveis esperados

1. Documento de auditoria com ocorrências encontradas e classificação de cada uma.
2. Correções de código necessárias para remover categorias fixas/pré-definidas.
3. Correções de código necessárias para remover `status` de categoria de nota.
4. Correções de código necessárias para separar `materias_chave` da criação de curso.
5. Nova rota ou rota ajustada para configurar `materias_chave` em curso médio.
6. Testes atualizados e novos testes para o contrato corrigido.
7. `docs/Spuri - Documentação.md` atualizado.
8. `docs/Spuri - API.md` atualizado.
9. Resumo final com comandos executados e arquivos alterados.
