import Foundation

/// A GCP gcloud configuration
public struct GCPConfiguration: Identifiable, Hashable, Sendable {
    public var id: String { name }
    public let name: String
    public var isActive: Bool
    public let project: String?
    public let account: String?
    public let region: String?

    /// Display name: project name if available, otherwise config name
    public var displayName: String {
        project ?? name
    }

    public init(name: String, isActive: Bool, project: String?, account: String?, region: String?) {
        self.name = name
        self.isActive = isActive
        self.project = project
        self.account = account
        self.region = region
    }
}

/// A GCP project discoverable via `gcloud projects list`
public struct GCPProject: Identifiable, Hashable, Sendable {
    public let projectId: String
    public let name: String
    public let projectNumber: String?

    public var id: String { projectId }

    public init(projectId: String, name: String, projectNumber: String?) {
        self.projectId = projectId
        self.name = name
        self.projectNumber = projectNumber
    }
}

/// A GCP authenticated account from `gcloud auth list`
public struct GCPAccount: Identifiable, Hashable, Sendable {
    public let account: String
    public let isActive: Bool

    public var id: String { account }

    public init(account: String, isActive: Bool) {
        self.account = account
        self.isActive = isActive
    }
}

// MARK: - JSON Decoding Helpers

/// JSON structure from `gcloud config configurations list --format=json`
public struct GCPConfigurationJSON: Decodable, Sendable {
    public let name: String
    public let is_active: Bool
    public let properties: GCPConfigProperties?

    public struct GCPConfigProperties: Decodable, Sendable {
        public let core: GCPCoreProperties?
        public let compute: GCPComputeProperties?
    }

    public struct GCPCoreProperties: Decodable, Sendable {
        public let project: String?
        public let account: String?
    }

    public struct GCPComputeProperties: Decodable, Sendable {
        public let region: String?
    }

    public func toConfiguration() -> GCPConfiguration {
        GCPConfiguration(
            name: name,
            isActive: is_active,
            project: properties?.core?.project,
            account: properties?.core?.account,
            region: properties?.compute?.region
        )
    }
}

/// JSON structure from `gcloud projects list --format=json`
public struct GCPProjectJSON: Decodable, Sendable {
    public let projectId: String
    public let name: String
    public let projectNumber: String?

    public func toProject() -> GCPProject {
        GCPProject(
            projectId: projectId,
            name: name,
            projectNumber: projectNumber
        )
    }
}

/// JSON structure from `gcloud auth list --format=json`
public struct GCPAccountJSON: Decodable, Sendable {
    public let account: String
    public let status: String

    public func toAccount() -> GCPAccount {
        GCPAccount(
            account: account,
            isActive: status == "ACTIVE"
        )
    }
}
