// Mapped type for flexible plugin configuration
export type PluginSettings = {
  [K in 'timeout' | 'retries']?: K extends 'timeout' ? number : number;
};

// A simple Method Decorator
function LogExecution(target: any, propertyKey: string, descriptor: PropertyDescriptor) {
  const originalMethod = descriptor.value;
  descriptor.value = function (...args: any[]) {
    console.log(`Executing "${propertyKey}"...`);
    const result = originalMethod.apply(this, args);
    console.log(`Finished executing "${propertyKey}".`);
    return result;
  };
  return descriptor;
}

export interface IPlugin {
  name: string;
  execute(data: any): void;
}

export class PluginManager {
  private plugins: Map<string, IPlugin> = new Map();

  constructor(private settings: PluginSettings = {}) {}

  @LogExecution
  public registerPlugin(plugin: IPlugin): void {
    if (this.plugins.has(plugin.name)) {
      throw new Error(`Plugin "${plugin.name}" is already registered.`);
    }
    console.log(`Registering plugin: ${plugin.name} with settings`, this.settings);
    this.plugins.set(plugin.name, plugin);
  }

  public runAll(data: any): void {
    this.plugins.forEach(plugin => {
      try {
        plugin.execute(data);
      } catch (error) {
        console.error(`Plugin ${plugin.name} failed`, error);
      }
    });
  }
}