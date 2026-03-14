import Foundation

/// A GCP gcloud configuration
struct GCPConfiguration: Identifiable, Hashable, Sendable {
    var id: String { name }
    let name: String
    var isActive: Bool
    let project: String?
    let account: String?
    let region: String?

    /// Display name: project name if available, otherwise config name
    var displayName: String {
        project ?? name
    }
}

/// A GCP project discoverable via `gcloud projects list`
struct GCPProject: Identifiable, Hashable, Sendable {
    let projectId: String
    let name: String
    let projectNumber: String?

    var id: String { projectId }
}

/// A GCP authenticated account from `gcloud auth list`
struct GCPAccount: Identifiable, Hashable, Sendable {
    let account: String
    let isActive: Bool

    var id: String { account }
}

// MARK: - JSON Decoding Helpers

/// JSON structure from `gcloud config configurations list --format=json`
struct GCPConfigurationJSON: Decodable, Sendable {
    let name: String
    let is_active: Bool
    let properties: GCPConfigProperties?

    struct GCPConfigProperties: Decodable, Sendable {
        let core: GCPCoreProperties?
        let compute: GCPComputeProperties?
    }

    struct GCPCoreProperties: Decodable, Sendable {
        let project: String?
        let account: String?
    }

    struct GCPComputeProperties: Decodable, Sendable {
        let region: String?
    }

    func toConfiguration() -> GCPConfiguration {
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
struct GCPProjectJSON: Decodable, Sendable {
    let projectId: String
    let name: String
    let projectNumber: String?

    func toProject() -> GCPProject {
        GCPProject(
            projectId: projectId,
            name: name,
            projectNumber: projectNumber
        )
    }
}

/// JSON structure from `gcloud auth list --format=json`
struct GCPAccountJSON: Decodable, Sendable {
    let account: String
    let status: String

    func toAccount() -> GCPAccount {
        GCPAccount(
            account: account,
            isActive: status == "ACTIVE"
        )
    }
}
