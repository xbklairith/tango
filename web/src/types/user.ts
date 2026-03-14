export interface AuthUser {
  id: string;
  email: string;
  displayName: string;
  status: "active" | "disabled";
  squads: AuthSquadMembership[];
}

export interface AuthSquadMembership {
  squadId: string;
  squadName: string;
  role: "owner" | "admin" | "viewer";
}
