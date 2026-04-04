// lib/email/email-service.ts

import nodemailer from 'nodemailer';
import type { Transporter } from 'nodemailer';

export interface EmailOptions {
  to: string | string[];
  subject: string;
  text?: string;
  html?: string;
  attachments?: Array<{
    filename: string;
    path?: string;
    content?: string | Buffer;
  }>;
}

export interface EmailConfig {
  host: string;
  port: number;
  secure: boolean;
  auth: {
    user: string;
    pass: string;
  };
}

class EmailService {
  private transporter: Transporter | null = null;
  private config: EmailConfig | null = null;

  /**
   * Inicializa o serviço de e-mail com as configurações do Gmail
   */
  initialize(config?: EmailConfig) {
    const emailConfig: EmailConfig = config || {
      host: process.env.EMAIL_HOST || 'smtp.gmail.com',
      port: parseInt(process.env.EMAIL_PORT || '587'),
      secure: process.env.EMAIL_SECURE === 'true', // true para 465, false para outros
      auth: {
        user: process.env.EMAIL_USER || '',
        pass: process.env.EMAIL_PASS || '', // App Password do Gmail
      },
    };

    this.config = emailConfig;

    // Configuração do transporter com opções para desenvolvimento
    this.transporter = nodemailer.createTransport({
      host: emailConfig.host,
      port: emailConfig.port,
      secure: emailConfig.secure,
      auth: {
        user: emailConfig.auth.user,
        pass: emailConfig.auth.pass,
      },
      // Aceitar certificados auto-assinados em desenvolvimento
      tls: {
        rejectUnauthorized: process.env.NODE_ENV === 'production',
      },
    });
  }

  /**
   * Verifica se o serviço está configurado corretamente
   */
  async verifyConnection(): Promise<boolean> {
    if (!this.transporter) {
      throw new Error('Email service not initialized. Call initialize() first.');
    }

    try {
      await this.transporter.verify();
      return true;
    } catch (error) {
      return false;
    }
  }

  /**
   * Envia um e-mail
   */
  async sendEmail(options: EmailOptions): Promise<{ success: boolean; messageId?: string; error?: string }> {
    if (!this.transporter) {
      return {
        success: false,
        error: 'Email service not initialized. Call initialize() first.',
      };
    }

    if (!this.config?.auth.user) {
      return {
        success: false,
        error: 'Email sender not configured. Check EMAIL_USER environment variable.',
      };
    }

    try {
      const info = await this.transporter.sendMail({
        from: `"Spuri" <${this.config.auth.user}>`,
        to: Array.isArray(options.to) ? options.to.join(', ') : options.to,
        subject: options.subject,
        text: options.text,
        html: options.html,
        attachments: options.attachments,
      });

      return {
        success: true,
        messageId: info.messageId,
      };
    } catch (error: any) {
      return {
        success: false,
        error: error.message || 'Unknown error occurred',
      };
    }
  }

  /**
   * Template para e-mail de verificação
   */
  async sendVerificationEmail(to: string, token: string, userName: string): Promise<{ success: boolean; messageId?: string; error?: string }> {
    const verificationUrl = `${process.env.NEXT_PUBLIC_APP_URL}/verificar-email/${token}`;

    const html = `
      <html>
        <head>
          <style>
            body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
            .container { max-width: 600px; margin: 0 auto; padding: 20px; }
            .header { background-color: #4F46E5; color: white; padding: 20px; text-align: center; border-radius: 8px 8px 0 0; }
            .content { background-color: #f9fafb; padding: 30px; border-radius: 0 0 8px 8px; }
            .button { display: inline-block; padding: 12px 30px; background-color: #4F46E5; color: white; text-decoration: none; border-radius: 6px; margin: 20px 0; }
            .footer { text-align: center; margin-top: 20px; font-size: 12px; color: #6b7280; }
          </style>
        </head>
        <body>
          <div class="container">
            <div class="header">
              <h1>Verificação de E-mail</h1>
            </div>
            <div class="content">
              <p>Olá, <strong>${userName}</strong>!</p>
              <p>Obrigado por se registrar no Spuri. Para complementar o seu cadastro, precisamos verificar o seu endereço de e-mail.</p>
              <p>Clique no botão abaixo para verificar sua conta:</p>
              <div style="text-align: center;">
                <a href="${verificationUrl}" style="display: inline-block; padding: 12px 30px; background-color: #4F46E5; color: #ffffff !important; text-decoration: none; border-radius: 6px; margin: 20px 0; font-family: Arial, sans-serif; font-size: 16px; font-weight: bold;">Verificar E-mail</a>
              </div>
              <p>Ou copie e cole este link no seu navegador:</p>
              <p style="background-color: #e5e7eb; padding: 10px; border-radius: 4px; word-break: break-all;">
                ${verificationUrl}
              </p>
              <p><strong>Este link expira em 24 horas.</strong></p>
              <p>Se você não solicitou esta verificação, por favor ignore este e-mail.</p>
            </div>
            <div class="footer">
              <p>© ${new Date().getFullYear()} Spuri - Todos os direitos reservados</p>
            </div>
          </div>
        </body>
      </html>
    `;

    return this.sendEmail({
      to,
      subject: 'Verificação de E-mail - Spuri',
      html,
      text: `Olá ${userName}! Para verificar seu e-mail, acesse: ${verificationUrl}`,
    });
  }

  /**
   * Template para e-mail de recuperação de senha
   * Inclui a senha padrão gerada pelo backend
   */
  async sendPasswordResetEmail(
    to: string, 
    token: string, 
    userName: string,
    senhaPadrao: string
  ): Promise<{ success: boolean; messageId?: string; error?: string }> {
    const resetUrl = `${process.env.NEXT_PUBLIC_APP_URL}/recuperar-senha/${token}`;

    const html = `
      <html>
        <head>
          <style>
            body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
            .container { max-width: 600px; margin: 0 auto; padding: 20px; }
            .header { background-color: #DC2626; color: white; padding: 20px; text-align: center; border-radius: 8px 8px 0 0; }
            .content { background-color: #f9fafb; padding: 30px; border-radius: 0 0 8px 8px; }
            .button { display: inline-block; padding: 12px 30px; background-color: #DC2626; color: white; text-decoration: none; border-radius: 6px; margin: 20px 0; }
            .warning { background-color: #FEF3C7; border-left: 4px solid #F59E0B; padding: 12px; margin: 20px 0; }
            .password-box { background-color: #DBEAFE; border: 2px solid #3B82F6; padding: 15px; margin: 20px 0; text-align: center; border-radius: 8px; }
            .password { font-size: 24px; font-weight: bold; color: #1E40AF; font-family: monospace; letter-spacing: 2px; }
            .footer { text-align: center; margin-top: 20px; font-size: 12px; color: #6b7280; }
          </style>
        </head>
        <body>
          <div class="container">
            <div class="header">
              <h1>Recuperação de Senha</h1>
            </div>
            <div class="content">
              <p>Olá, <strong>${userName}</strong>!</p>
              <p>Sua senha foi resetada com sucesso. Use a senha padrão abaixo para fazer login:</p>
              
              <div class="password-box">
                <p style="margin: 0 0 10px 0; font-size: 14px; color: #4B5563;">Sua nova senha temporária:</p>
                <div class="password">${senhaPadrao}</div>
              </div>

              <div class="warning">
                <strong>⚠️ Importante:</strong> Por motivos de segurança, altere esta senha imediatamente após fazer login. Acesse seu perfil e vá em "Alterar Senha".
              </div>

              <div class="warning">
                <strong>🔒 Dicas de Segurança:</strong>
                <ul style="margin: 10px 0; padding-left: 20px;">
                  <li>Nunca compartilhe sua senha com ninguém</li>
                  <li>Use uma senha forte com letras, números e símbolos</li>
                  <li>Não use a mesma senha em diferentes serviços</li>
                </ul>
              </div>

              <p>Se você não solicitou a recuperação de senha, entre em contato conosco imediatamente.</p>
            </div>
            <div class="footer">
              <p>© ${new Date().getFullYear()} Spuri - Todos os direitos reservados</p>
              <p>Se você tiver problemas, entre em contato com nosso suporte.</p>
            </div>
          </div>
        </body>
      </html>
    `;

    const textContent = `
Olá ${userName}!

Sua senha foi resetada com sucesso.

SENHA TEMPORÁRIA: ${senhaPadrao}

IMPORTANTE: Altere esta senha imediatamente após fazer login por motivos de segurança.

Você pode acessar a página de alteração de senha através deste link:
${resetUrl}

Se você não solicitou esta recuperação, entre em contato conosco imediatamente.

---
© ${new Date().getFullYear()} Spuri - Todos os direitos reservados
    `;

    return this.sendEmail({
      to,
      subject: 'Senha Resetada - Spuri',
      html,
      text: textContent,
    });
  }

}

// Singleton instance
export const emailService = new EmailService();