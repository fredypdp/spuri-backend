---
criado: 2026-07-09 00:00
origem: solicitação do usuário
status: feito
---

# Atualizar validação de declaração acadêmica na matrícula do estudante (feito)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que, no cadastro direto de estudante pela academia e na solicitação de matrícula, a validação de documentos acadêmicos cobre declaração em todos os anos escolares aplicáveis, exigindo que a declaração pertença obrigatoriamente ao ano acadêmico anterior ao ano em que o estudante está sendo cadastrado ou matriculado. Manter a exceção do `1_ano_fundamental`, em que não se cobra certificado nem declaração, e manter os anos em que o certificado é obrigatório com declaração aceita como alternativa, desde que essa declaração também seja do ano acadêmico imediatamente anterior. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade, fallbacks temporários ou exceções não documentadas.

## Contexto

A validação de documentos acadêmicos precisa garantir rastreabilidade entre o ano acadêmico pretendido pelo estudante e o comprovativo apresentado. A declaração escolar deixa de ser apenas um documento genérico e passa a ter vínculo obrigatório com o ano acadêmico anterior ao ingresso, evitando que declarações de anos incompatíveis sejam usadas para concluir cadastro ou solicitação de matrícula.

A regra deve valer tanto para o cadastro direto realizado pela academia quanto para a solicitação de matrícula feita pelo estudante ou responsável. O `1_ano_fundamental` permanece como exceção porque não possui ano acadêmico anterior exigível dentro do fluxo escolar. Nos anos de ingresso em que o certificado é aceito como comprovação principal e a declaração é aceita como alternativa, a alternativa por declaração também deve demonstrar o ano acadêmico imediatamente anterior ao ano de ingresso.

## Resumo executivo

| Item                  | Decisão                                                  | Resultado esperado                                                               |
| --------------------- | -------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Declaração escolar    | Obrigatória em todos os anos escolares aplicáveis        | Exigir declaração quando não houver regra de certificado obrigatório/alternativo |
| Ano da declaração     | Sempre o ano acadêmico anterior ao ingresso              | Rejeitar declaração de ano diferente do anterior ao ano pretendido               |
| `1_ano_fundamental`   | Exceção total para comprovativo acadêmico anterior       | Não cobrar certificado nem declaração                                            |
| Anos com certificado  | Certificado continua válido conforme regra vigente       | Aceitar certificado exigido ou declaração alternativa do ano acadêmico anterior  |
| Fluxos afetados       | Solicitação de matrícula e cadastro direto pela academia | Aplicar a mesma regra nos dois caminhos                                          |
| Documentação e testes | Atualização obrigatória                                  | Contratos, OpenAPI/Swagger e cenários automatizados devem refletir a regra       |

---

# 1. Atualizar regra geral de cobrança da declaração

## Objetivo

Garantir que todo estudante do nível escolar, exceto no `1_ano_fundamental` e nos casos em que certificado válido substitua a declaração, apresente declaração referente ao ano acadêmico imediatamente anterior ao ano em que está ingressando.

## Regra de negócio

Ao criar uma solicitação de matrícula ou cadastrar diretamente um estudante pela academia, o backend deve:

1. identificar o ano acadêmico pretendido do estudante;
2. calcular o ano acadêmico imediatamente anterior ao ano pretendido;
3. exigir declaração desse ano anterior em todos os anos escolares aplicáveis;
4. rejeitar declaração sem metadado, campo ou classificação que permita confirmar o ano acadêmico de referência;
5. rejeitar declaração associada a ano acadêmico diferente do ano anterior exigido;
6. manter a exceção explícita do `1_ano_fundamental`, sem cobrança de certificado ou declaração;
7. manter os anos em que certificado é obrigatório e declaração é aceita como alternativa, validando também nesses casos que a declaração alternativa seja do ano anterior;
8. retornar erro padronizado quando o documento obrigatório estiver ausente, inválido ou incompatível;
9. aplicar exatamente a mesma regra em solicitação de matrícula e cadastro direto pela academia.

## Escopo obrigatório

### 1.1 Calcular o ano acadêmico anterior

Atualizar ou criar regra centralizada para determinar o ano acadêmico imediatamente anterior ao ano pretendido.

A implementação deve evitar listas duplicadas e divergentes sempre que o projeto possuir catálogo, enum, configuração ou tabela de progressão dos anos acadêmicos.

### 1.2 Exigir declaração nos anos escolares aplicáveis

Para anos escolares que possuam ano anterior e não estejam cobertos por certificado obrigatório enviado e válido, o backend deve exigir declaração do ano acadêmico anterior.

Exemplos esperados:

1. matrícula/cadastro no `2_ano_fundamental` deve exigir declaração do `1_ano_fundamental`;
2. matrícula/cadastro no `3_ano_fundamental` deve exigir declaração do `2_ano_fundamental`;
3. matrícula/cadastro em qualquer outro ano escolar sequencial deve exigir declaração do ano imediatamente anterior, salvo regra específica de certificado documentada nesta tarefa;
4. declaração de ano posterior, do mesmo ano pretendido ou de ano anterior não imediato deve ser rejeitada.

### 1.3 Manter exceção do `1_ano_fundamental`

Para matrícula ou cadastro no `1_ano_fundamental`:

1. não exigir declaração;
2. não exigir certificado;
3. não tentar calcular comprovativo acadêmico anterior inexistente;
4. manter apenas as demais validações de documentos pessoais, identificação e autorização aplicáveis ao nível escolar.

### 1.4 Validar anos em que certificado é obrigatório com declaração alternativa

Nos anos escolares em que a regra vigente cobra certificado e aceita declaração como alternativa, o backend deve:

1. manter o certificado válido como documento suficiente quando o certificado enviado corresponder ao ano/conclusão exigida;
2. aceitar declaração apenas como alternativa ao certificado;
3. exigir que a declaração alternativa seja do ano acadêmico imediatamente anterior ao ano pretendido;
4. rejeitar declaração alternativa sem identificação do ano acadêmico;
5. rejeitar declaração alternativa de ano diferente do anterior exigido;
6. rejeitar a operação quando não houver certificado válido nem declaração alternativa válida.

Exemplos esperados conforme regras já documentadas:

1. matrícula/cadastro no `7_ano_fundamental`: aceitar certificado válido do `6_ano_fundamental` ou declaração do `6_ano_fundamental`;
2. matrícula/cadastro no `1_ano_medio`: aceitar certificado válido do `9_ano_fundamental` ou declaração do `9_ano_fundamental`;
3. outros anos de transição com certificado obrigatório devem seguir o mesmo princípio caso existam no catálogo do sistema.

---

# 2. Unificar solicitação de matrícula e cadastro direto pela academia

## Objetivo

Evitar divergência entre os fluxos de admissão de estudante e garantir que a mesma regra documental seja aplicada independentemente da origem da matrícula.

## Regra de negócio

As validações de declaração e certificado devem ser equivalentes em:

1. solicitação de matrícula criada pelo estudante ou responsável;
2. cadastro direto de estudante feito pela academia;
3. aprovação ou conversão de solicitação em estudante, quando esse fluxo revalida documentos;
4. atualização ou complementação documental durante o processo de admissão, quando aplicável.

## Escopo obrigatório

### 2.1 Centralizar regras documentais

Auditar handlers, validators, DTOs, commands, serviços, policies e casos de uso relacionados a estudantes para localizar regras duplicadas de obrigatoriedade documental.

Preferir uma única função, policy, serviço de domínio ou helper compartilhado para determinar:

1. se o ano pretendido exige declaração;
2. qual é o ano acadêmico esperado da declaração;
3. se certificado pode substituir a declaração;
4. qual certificado é aceito para o ano pretendido;
5. qual erro deve ser retornado em cada falha.

### 2.2 Garantir consistência de mensagens

As mensagens e códigos de erro devem indicar claramente:

1. quando a declaração está ausente;
2. quando a declaração pertence a ano acadêmico incorreto;
3. qual ano acadêmico era esperado;
4. quando certificado válido poderia substituir a declaração;
5. quando nenhum documento acadêmico anterior é exigido, como no `1_ano_fundamental`;
6. o fluxo em que a validação falhou, sem expor detalhes sensíveis.

---

# 3. Atualizar contratos, metadados e documentação de API

## Objetivo

Garantir que os contratos públicos expressem claramente a relação obrigatória entre declaração e ano acadêmico anterior.

## Escopo obrigatório

Atualizar, quando existirem:

1. DTOs e schemas de request usados para documentos acadêmicos;
2. serializers e schemas de response que exponham status documental;
3. OpenAPI/Swagger;
4. documentação técnica de matrícula e cadastro de estudantes;
5. exemplos de payload multipart/form-data;
6. coleções de API;
7. guias operacionais de academias;
8. documentos de tarefas anteriores usados como referência ativa.

## Regras de documentação

A documentação deve declarar explicitamente que:

1. a declaração deve ser do ano acadêmico imediatamente anterior ao ano pretendido;
2. `1_ano_fundamental` não cobra certificado nem declaração;
3. nos anos em que certificado é exigido e declaração é alternativa, a declaração alternativa também deve ser do ano acadêmico anterior;
4. declaração sem ano acadêmico identificável deve ser rejeitada;
5. solicitação de matrícula e cadastro direto pela academia seguem a mesma regra.

---

# 4. Atualizar testes automatizados

## Objetivo

Garantir cobertura suficiente para impedir regressões na validação de declaração, certificado e ano acadêmico anterior.

## Cenários obrigatórios

Adicionar ou ajustar testes cobrindo:

1. `1_ano_fundamental` aceitando solicitação de matrícula sem declaração;
2. `1_ano_fundamental` aceitando cadastro direto sem declaração;
3. `1_ano_fundamental` aceitando solicitação de matrícula sem certificado;
4. `1_ano_fundamental` aceitando cadastro direto sem certificado;
5. ano escolar sequencial exigindo declaração do ano imediatamente anterior na solicitação de matrícula;
6. ano escolar sequencial exigindo declaração do ano imediatamente anterior no cadastro direto;
7. rejeição de declaração do mesmo ano pretendido;
8. rejeição de declaração de ano posterior;
9. rejeição de declaração de ano anterior não imediato;
10. rejeição de declaração sem ano acadêmico identificável;
11. ano com certificado obrigatório aceitando certificado válido;
12. ano com certificado obrigatório aceitando declaração alternativa do ano anterior;
13. ano com certificado obrigatório rejeitando declaração alternativa de ano incorreto;
14. rejeição quando não houver certificado válido nem declaração válida nos anos que exigem um dos dois;
15. mensagens de erro informando o ano acadêmico esperado;
16. OpenAPI/Swagger ou snapshots de contrato refletindo a regra atualizada.

---

# 5. Fora de escopo

- Cobrar certificado ou declaração no `1_ano_fundamental`.
- Aceitar declaração de ano acadêmico diferente do imediatamente anterior ao ano pretendido.
- Aceitar declaração sem metadado, campo ou classificação suficiente para validar o ano acadêmico de referência.
- Manter regras divergentes entre solicitação de matrícula e cadastro direto pela academia.
- Criar fallback para regra antiga de declaração genérica sem ano acadêmico.
- Criar aliases, wrappers de compatibilidade ou exceções temporárias para documentos inválidos.
- Alterar regras de identificação pessoal do estudante ou responsável que não estejam diretamente relacionadas à validação da declaração acadêmica.
- Alterar regras de progressão acadêmica fora do necessário para calcular o ano acadêmico anterior.

---

# 6. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. solicitação de matrícula exigir declaração do ano acadêmico anterior em todos os anos escolares aplicáveis;
2. cadastro direto de estudante exigir declaração do ano acadêmico anterior em todos os anos escolares aplicáveis;
3. `1_ano_fundamental` não cobrar certificado nem declaração;
4. declaração de ano diferente do imediatamente anterior for rejeitada;
5. declaração sem ano acadêmico identificável for rejeitada;
6. anos com certificado obrigatório aceitarem certificado válido conforme regra vigente;
7. anos com certificado obrigatório aceitarem declaração alternativa somente quando ela for do ano acadêmico anterior;
8. anos com certificado obrigatório rejeitarem declaração alternativa de ano incorreto;
9. regras documentais estiverem centralizadas ou compartilhadas para evitar divergência entre fluxos;
10. respostas de erro estiverem padronizadas e indicarem o documento/ano esperado;
11. OpenAPI/Swagger, documentação técnica e exemplos estiverem atualizados;
12. testes automatizados cobrirem os cenários de exceção, sucesso e rejeição descritos nesta tarefa;
13. o PR explicar claramente a mudança de regra, os fluxos afetados e os impactos nos contratos documentais.
