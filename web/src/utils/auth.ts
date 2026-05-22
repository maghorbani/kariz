import type { UserProfile } from '@/types';

export const ALL_ROLES = ['admin', 'qa', 'data', 'operations'] as const;

export const ROLE_COLORS: Record<string, string> = {
  admin: 'red',
  qa: 'blue',
  data: 'green',
  operations: 'orange',
};

export function isAdmin(user: UserProfile | null): boolean {
  return user?.roles.includes('admin') ?? false;
}
