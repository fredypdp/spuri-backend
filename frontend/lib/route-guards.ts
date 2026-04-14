// src/lib/route-guards.ts

import { UserType } from '@/types/api';

export interface RouteConfig {
  path: string;
  allowedTypes: UserType[] | 'public' | 'authenticated';
  redirectIfUnauthorized?: string;
}

export const ROUTE_PERMISSIONS: RouteConfig[] = [
  // ==========================================
  // ROTAS PÚBLICAS (sem autenticação)
  // ==========================================
  {
    path: '/login',
    allowedTypes: 'public',
    redirectIfUnauthorized: '/',
  },
  {
    path: '/esqueci-senha',
    allowedTypes: 'public',
    redirectIfUnauthorized: '/',
  },
  {
    path: '/dominis/esqueci-senha',
    allowedTypes: 'public',
    redirectIfUnauthorized: '/',
  },
  {
    path: '/verificar-email',
    allowedTypes: 'public',
  },
  {
    path: '/error-404',
    allowedTypes: 'public',
  },

  // ==========================================
  // ROTAS APENAS PARA ADMIN
  // ==========================================
  {
    path: '/academias',
    allowedTypes: ['admin'],
    redirectIfUnauthorized: '/',
  },
  {
    path: '/estudantes',
    allowedTypes: ['admin', 'academia'],
    redirectIfUnauthorized: '/',
  },
  {
    path: '/configuracoes',
    allowedTypes: ['admin', 'academia'],
    redirectIfUnauthorized: '/',
  },

  // ==========================================
  // ROTAS PARA ACADEMIA — Gerenciamento (sub-páginas)
  // ==========================================
  {
    path: '/gerenciamento/cursos',
    allowedTypes: ['academia'],
    redirectIfUnauthorized: '/',
  },
  {
    path: '/gerenciamento/materias-disciplinares',
    allowedTypes: ['academia'],
    redirectIfUnauthorized: '/',
  },
  {
    path: '/gerenciamento/turmas',
    allowedTypes: ['academia'],
    redirectIfUnauthorized: '/',
  },
  {
    path: '/gerenciamento',
    allowedTypes: ['academia'],
    redirectIfUnauthorized: '/',
  },

  // ==========================================
  // ROTAS DE TESTES — apenas academia
  // ==========================================
  {
    path: '/dev/seed',
    allowedTypes: ['academia'],
    redirectIfUnauthorized: '/',
  },

  // ==========================================
  // NOTAS / FALTAS
  // ==========================================
  {
    path: '/notas',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },
  {
    path: '/faltas',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },

  // ==========================================
  // ROTAS DE AVALIAÇÕES
  // ==========================================
  {
    path: '/avaliacoes',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },
  {
    path: '/avaliacoes/avaliacoes-finais',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },

  // ==========================================
  // ROTAS PARA USUÁRIOS AUTENTICADOS
  // ==========================================
  {
    path: '/perfil',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },
  {
    path: '/',
    allowedTypes: 'authenticated',
    redirectIfUnauthorized: '/login',
  },
];

export function checkRoutePermission(
  pathname: string,
  userType: UserType | null,
  isAuthenticated: boolean
): {
  allowed: boolean;
  redirectTo?: string;
} {
  const normalizedPath = pathname.endsWith('/') && pathname !== '/'
    ? pathname.slice(0, -1)
    : pathname;

  const routeConfig = ROUTE_PERMISSIONS.find(route => {
    if (route.path === normalizedPath) return true;
    if (route.path.includes('[') && normalizedPath.startsWith(route.path.split('[')[0])) {
      return true;
    }
    return false;
  });

  if (!routeConfig) {
    if (!isAuthenticated) {
      return { allowed: false, redirectTo: '/login' };
    }
    return { allowed: true };
  }

  if (routeConfig.allowedTypes === 'public') {
    if (isAuthenticated && routeConfig.redirectIfUnauthorized) {
      return { allowed: false, redirectTo: routeConfig.redirectIfUnauthorized };
    }
    return { allowed: true };
  }

  if (routeConfig.allowedTypes === 'authenticated') {
    if (!isAuthenticated) {
      return { allowed: false, redirectTo: routeConfig.redirectIfUnauthorized || '/login' };
    }
    return { allowed: true };
  }

  if (Array.isArray(routeConfig.allowedTypes)) {
    if (!isAuthenticated || !userType) {
      return { allowed: false, redirectTo: routeConfig.redirectIfUnauthorized || '/login' };
    }
    if (!routeConfig.allowedTypes.includes(userType)) {
      return { allowed: false, redirectTo: routeConfig.redirectIfUnauthorized || '/' };
    }
    return { allowed: true };
  }

  return { allowed: false, redirectTo: '/login' };
}

export function getRouteInfo(pathname: string): RouteConfig | null {
  const normalizedPath = pathname.endsWith('/') && pathname !== '/'
    ? pathname.slice(0, -1)
    : pathname;

  return ROUTE_PERMISSIONS.find(route => {
    if (route.path === normalizedPath) return true;
    if (route.path.includes('[') && normalizedPath.startsWith(route.path.split('[')[0])) {
      return true;
    }
    return false;
  }) || null;
}