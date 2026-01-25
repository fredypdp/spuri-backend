-- Adicionar campo email_verificado para admins
ALTER TABLE projection_admins 
ADD COLUMN IF NOT EXISTS email_verificado BOOLEAN DEFAULT FALSE;

-- Atualizar admins existentes para ter email verificado como false
UPDATE projection_admins 
SET email_verificado = FALSE 
WHERE email_verificado IS NULL;

COMMENT ON COLUMN projection_admins.email_verificado IS 'Se o email do admin foi verificado';