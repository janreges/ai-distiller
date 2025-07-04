export class UserModel {
  public readonly id: number;
  private lastLogin: Date | null = null;

  /**
   * Creates a new user model.
   * @param id The unique user ID.
   * @param username The user's public name.
   * @param email The user's private email address.
   */
  constructor(id: number, public username: string, private email: string) {
    this.id = id;
    this._validateEmail(email);
  }

  public getProfileInfo() {
    return {
      username: this.username,
      profileUrl: this._generateProfileUrl(),
    };
  }

  public recordLogin(): void {
    this.lastLogin = new Date();
  }

  private _validateEmail(email: string): void {
    if (!email.includes('@')) {
      throw new Error("Invalid email address provided.");
    }
  }

  private _generateProfileUrl(): string {
    return `/users/${this.username.toLowerCase()}`;
  }
}