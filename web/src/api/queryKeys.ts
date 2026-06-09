/**
 * Typed React Query key factory. One namespace per feature area; keys are tuples
 * so a partial key can target invalidation — e.g.
 * `queryClient.invalidateQueries({ queryKey: qk.bases.all })` clears every bases
 * query. Extend this as each tab migrates from the hand-rolled fetch pattern
 * onto `useQuery`/`useMutation`.
 */
export const qk = {
  bases: {
    all: ['bases'] as const,
    list: ['bases', 'list'] as const,
  },
  landsraad: {
    all: ['landsraad'] as const,
    overview: ['landsraad', 'overview'] as const,
  },
} as const
