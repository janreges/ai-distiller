const http = require('./http.cjs');

export class ApiClient {
  get(url: string): string {
    return http.get(url);
  }
}
