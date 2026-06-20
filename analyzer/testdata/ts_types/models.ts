export interface Serializable {
    serialize(): string;
    deserialize(data: string): void;
}

export interface Printable {
    print(): void;
}

export interface Loggable extends Serializable, Printable {
    log(level: string): void;
}

export class Animal {
    name: string;
    age: number;

    constructor(name: string, age: number) {
        this.name = name;
        this.age = age;
    }

    speak(): string {
        return "";
    }
}

export class Dog extends Animal implements Serializable {
    breed: string;

    constructor(name: string, age: number, breed: string) {
        super(name, age);
        this.breed = breed;
    }

    speak(): string {
        return "Woof";
    }

    serialize(): string {
        return JSON.stringify(this);
    }

    deserialize(data: string): void {}
}

export class GuideDog extends Dog implements Loggable {
    guide(destination: string): void {}
    log(level: string): void {}
    print(): void {}
}

abstract class Shape {
    abstract area(): number;
    abstract perimeter(): number;
}

class Circle extends Shape {
    constructor(private radius: number) {
        super();
    }
    area(): number { return Math.PI * this.radius ** 2; }
    perimeter(): number { return 2 * Math.PI * this.radius; }
}

class _InternalHelper {
    helper(): void {}
}
